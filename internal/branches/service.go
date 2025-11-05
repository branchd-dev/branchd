package branches

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/branchd-dev/branchd/internal/assert"
	"github.com/branchd-dev/branchd/internal/config"
	"github.com/branchd-dev/branchd/internal/models"
)

// allowedPostgresqlSettings defines which PostgreSQL settings users can customize
var allowedPostgresqlSettings = map[string]bool{
	"max_connections":                 true,
	"max_parallel_workers":            true,
	"max_worker_processes":            true,
	"effective_io_concurrency":        true,
	"random_page_cost":                true,
	"shared_preload_libraries":        true,
	"max_parallel_workers_per_gather": true,
	"shared_buffers":                  true,
	"work_mem":                        true,
	"maintenance_work_mem":            true,
	"effective_cache_size":            true,
	"max_wal_size":                    true,
	"wal_buffers":                     true,

	// Not shown in UI but fine to allow
	"timezone":  true,
	"datestyle": true,
}

//go:embed create-branch.sh
var createBranchScript string

//go:embed destroy-branch.sh
var destroyBranchScript string

// filterPostgresqlSettings filters and validates user-provided PostgreSQL settings
func filterPostgresqlSettings(customConf string) (string, error) {
	if strings.TrimSpace(customConf) == "" {
		return "", nil
	}

	var filteredLines []string
	lines := strings.Split(customConf, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse setting: key = value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Check if setting is allowed
		if !allowedPostgresqlSettings[key] {
			continue // Skip disallowed settings silently
		}

		// Apply specific validation for certain settings
		if key == "max_connections" {
			if conn, err := strconv.Atoi(value); err != nil || conn < 1 || conn > 100 {
				continue // Skip invalid max_connections (limit to 100)
			}
		}

		// Add validated setting
		filteredLines = append(filteredLines, fmt.Sprintf("%s = %s", key, value))
	}

	result := strings.Join(filteredLines, "\n")
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}

type Service struct {
	db     *gorm.DB
	config *config.Config
	logger zerolog.Logger
}

type CreateBranchParams struct {
	BranchName  string
	CreatedByID string
}

type branchScriptParams struct {
	BranchName           string
	DatasetName          string
	User                 string
	Password             string
	PgVersion            string
	CustomPostgresqlConf string // base64-encoded custom settings
}

type deleteBranchScriptParams struct {
	BranchName  string
	DatasetName string
}

// ForcedBranchMetadata contains metadata to force during branch creation (used for refresh)
type ForcedBranchMetadata struct {
	Port     int
	User     string
	Password string
}

func NewService(db *gorm.DB, cfg *config.Config, logger zerolog.Logger) *Service {
	return &Service{
		db:     db,
		config: cfg,
		logger: logger.With().Str("component", "branches_service").Logger(),
	}
}

func (s *Service) CreateBranch(ctx context.Context, params CreateBranchParams) (*models.Branch, error) {
	s.logger.Info().
		Str("branch_name", params.BranchName).
		Str("created_by_id", params.CreatedByID).
		Msg("Creating new branch")

	// Load config (singleton)
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("configuration not found, please complete onboarding first")
		}
		s.logger.Error().Err(err).Msg("Failed to load config")
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Find the latest ready restore (must have ready_at set)
	var restore models.Restore
	if err := s.db.Where("schema_ready = ? AND ready_at IS NOT NULL", true).
		Order("ready_at DESC").
		First(&restore).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no ready restore found")
		}
		s.logger.Error().Err(err).Msg("Failed to load restore")
		return nil, fmt.Errorf("failed to load restore: %w", err)
	}

	// Check if branch already exists by name (branch names are unique)
	// If it exists, return it regardless of which restore it came from
	var existingBranch models.Branch
	err := s.db.Where("name = ?", params.BranchName).First(&existingBranch).Error
	if err == nil {
		s.logger.Info().
			Str("branch_id", existingBranch.ID).
			Str("branch_name", params.BranchName).
			Str("restore_id", existingBranch.RestoreID).
			Msg("Branch already exists, returning existing branch")
		return &existingBranch, nil
	} else if err != gorm.ErrRecordNotFound {
		s.logger.Error().Err(err).Str("branch_name", params.BranchName).Msg("Failed to check existing branch")
		return nil, fmt.Errorf("failed to check existing branch: %w", err)
	}

	// Generate credentials for new branch
	user, err := s.genRandomString(16)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate random user")
		return nil, fmt.Errorf("failed to generate random user: %w", err)
	}

	password, err := s.genRandomString(32)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to generate random password")
		return nil, fmt.Errorf("failed to generate random password: %w", err)
	}

	// Execute branch creation synchronously
	return s.executeBranchCreation(ctx, &config, &restore, params, user, password)
}

func (s *Service) executeBranchCreation(ctx context.Context, config *models.Config, restore *models.Restore, params CreateBranchParams, user, password string) (*models.Branch, error) {
	// Filter and encode custom PostgreSQL configuration
	filteredConf, err := filterPostgresqlSettings(config.BranchPostgresqlConf)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to filter PostgreSQL settings")
		return nil, fmt.Errorf("failed to filter PostgreSQL settings: %w", err)
	}

	var encodedConf string
	if filteredConf != "" {
		encodedConf = base64.StdEncoding.EncodeToString([]byte(filteredConf))
	}

	// Verify credentials length
	assert.Length(user, 16)     // 16-char user
	assert.Length(password, 32) // 32-char password

	// Execute branch creation script (includes ZFS clone, service start, user creation)
	// Use PostgreSQL version cluster dataset (e.g., tank/pg16)
	datasetName := fmt.Sprintf("tank/pg%s", config.PostgresVersion)
	scriptParams := branchScriptParams{
		BranchName:           params.BranchName,
		DatasetName:          datasetName,
		User:                 user,
		Password:             password,
		PgVersion:            config.PostgresVersion,
		CustomPostgresqlConf: encodedConf,
	}

	script, err := s.renderBranchScript(scriptParams)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to render branch creation script")
		return nil, fmt.Errorf("failed to render branch creation script: %w", err)
	}

	// Execute branch creation script locally
	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		// Check if output contains our custom error markers
		if strings.Contains(output, "BRANCHD_ERROR:DATABASE_NOT_READY") {
			errorMsg := extractErrorMessage(output)
			s.logger.Info().Str("branch_name", params.BranchName).Str("error_detail", errorMsg).Msg("Branch creation failed: source database not ready")
			return nil, fmt.Errorf("instance is still in initial recovery. Please wait a few minutes and try again")
		}
		if strings.Contains(output, "BRANCHD_ERROR:RESTORE_NOT_RUNNING") {
			errorMsg := extractErrorMessage(output)
			s.logger.Info().Str("branch_name", params.BranchName).Str("error_detail", errorMsg).Msg("Branch creation failed: restore process not running")
			return nil, fmt.Errorf("instance not ready: restore_not_running")
		}
		s.logger.Error().Err(err).Str("branch_name", params.BranchName).Str("output", output).Msg("Failed to execute branch creation script")
		return nil, fmt.Errorf("failed to execute branch creation script: %w", err)
	}

	// Verify user creation was successful
	if !strings.Contains(output, "USER_CREATION_SUCCESS=true") {
		s.logger.Error().Str("output", output).Msg("Branch creation script did not report success")
		return nil, fmt.Errorf("branch creation script failed")
	}

	// Parse port number from branch creation script output
	port, err := s.parseBranchPortFromOutput(output)
	if err != nil {
		s.logger.Error().Err(err).Str("output", output).Msg("Failed to parse port from script output")
		return nil, fmt.Errorf("failed to parse port from script output: %w", err)
	}

	// Create branch record in database (only after successful creation)
	branch := models.Branch{
		Name:        params.BranchName,
		RestoreID:   restore.ID,
		CreatedByID: params.CreatedByID,
		User:        user,
		Password:    password,
		Port:        port,
	}

	if err := s.db.Create(&branch).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to create branch record")
		return nil, fmt.Errorf("failed to create branch record: %w", err)
	}

	s.logger.Info().
		Str("branch_id", branch.ID).
		Str("branch_name", params.BranchName).
		Int("port", port).
		Msg("Branch created successfully")

	return &branch, nil
}

func (s *Service) renderBranchScript(params branchScriptParams) (string, error) {
	tmpl, err := template.New("create-branch").Parse(createBranchScript)
	if err != nil {
		return "", fmt.Errorf("failed to parse script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute script template: %w", err)
	}

	return buf.String(), nil
}

func (s *Service) parseBranchPortFromOutput(output string) (int, error) {
	// Look for BRANCH_PORT=<number> in the output
	re := regexp.MustCompile(`BRANCH_PORT=(\d+)`)
	matches := re.FindStringSubmatch(output)

	if len(matches) < 2 {
		return 0, fmt.Errorf("BRANCH_PORT not found in output")
	}

	port, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("failed to parse port number: %w", err)
	}

	return port, nil
}

func (s *Service) genRandomString(size int) (string, error) {
	// Calculate the number of bytes needed
	// Base64 encoding increases size by ~33%, so we need fewer bytes
	numBytes := (size * 3) / 4
	if (size*3)%4 != 0 {
		numBytes++
	}

	// Generate random bytes
	bytes := make([]byte, numBytes)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}

	// Encode to base64 and make URL-safe
	encoded := base64.URLEncoding.EncodeToString(bytes)

	// Remove padding and trim to desired length
	encoded = strings.TrimRight(encoded, "=")
	if len(encoded) > size {
		encoded = encoded[:size]
	}

	return encoded, nil
}

func extractErrorMessage(output string) string {
	re := regexp.MustCompile(`(BRANCHD_ERROR.*)`)
	matches := re.FindStringSubmatch(output)

	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	return "-- no error message --"
}

// CreateBranchWithForcedMetadata creates a branch with forced port/credentials (used during refresh)
func (s *Service) CreateBranchWithForcedMetadata(ctx context.Context, params CreateBranchParams, forced ForcedBranchMetadata) (*models.Branch, error) {
	s.logger.Info().
		Str("branch_name", params.BranchName).
		Int("forced_port", forced.Port).
		Msg("Creating branch with forced metadata (refresh)")

	// Load config (singleton)
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("configuration not found")
		}
		s.logger.Error().Err(err).Msg("Failed to load config")
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Find the latest ready restore (must have ready_at set)
	var restore models.Restore
	if err := s.db.Where("schema_ready = ? AND ready_at IS NOT NULL", true).
		Order("ready_at DESC").
		First(&restore).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no ready restore found")
		}
		s.logger.Error().Err(err).Msg("Failed to load restore")
		return nil, fmt.Errorf("failed to load restore: %w", err)
	}

	// Execute branch creation synchronously with forced port
	return s.executeBranchCreationWithForcedPort(ctx, &config, &restore, params, forced.User, forced.Password, forced.Port)
}

func (s *Service) executeBranchCreationWithForcedPort(ctx context.Context, config *models.Config, restore *models.Restore, params CreateBranchParams, user, password string, forcePort int) (*models.Branch, error) {
	// Filter and encode custom PostgreSQL configuration
	filteredConf, err := filterPostgresqlSettings(config.BranchPostgresqlConf)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to filter PostgreSQL settings")

		return nil, fmt.Errorf("failed to filter PostgreSQL settings: %w", err)
	}

	var encodedConf string
	if filteredConf != "" {
		encodedConf = base64.StdEncoding.EncodeToString([]byte(filteredConf))
	}

	// Verify credentials length
	assert.Length(user, 16)     // 16-char user
	assert.Length(password, 32) // 32-char password

	// Execute branch creation script with FORCE_PORT environment variable
	// Use PostgreSQL version cluster dataset (e.g., tank/pg16)
	datasetName := fmt.Sprintf("tank/pg%s", config.PostgresVersion)
	scriptParams := branchScriptParams{
		BranchName:           params.BranchName,
		DatasetName:          datasetName,
		User:                 user,
		Password:             password,
		PgVersion:            config.PostgresVersion,
		CustomPostgresqlConf: encodedConf,
	}

	script, err := s.renderBranchScript(scriptParams)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to render branch creation script")

		return nil, fmt.Errorf("failed to render branch creation script: %w", err)
	}

	// Execute branch creation script locally with FORCE_PORT environment variable
	scriptWithEnv := fmt.Sprintf("export FORCE_PORT=%d\n%s", forcePort, script)
	cmd := exec.CommandContext(ctx, "bash", "-c", scriptWithEnv)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		// Check if output contains our custom error markers
		if strings.Contains(output, "BRANCHD_ERROR:DATABASE_NOT_READY") {
			errorMsg := extractErrorMessage(output)
			s.logger.Info().Str("branch_name", params.BranchName).Str("error_detail", errorMsg).Msg("Branch creation failed: source database not ready")

			return nil, fmt.Errorf("instance is still in initial recovery. Please wait a few minutes and try again")
		}
		if strings.Contains(output, "BRANCHD_ERROR:RESTORE_NOT_RUNNING") {
			errorMsg := extractErrorMessage(output)
			s.logger.Info().Str("branch_name", params.BranchName).Str("error_detail", errorMsg).Msg("Branch creation failed: restore process not running")

			return nil, fmt.Errorf("instance not ready: restore_not_running")
		}
		s.logger.Error().Err(err).Str("branch_name", params.BranchName).Str("output", output).Msg("Failed to execute branch creation script with forced port")

		return nil, fmt.Errorf("failed to execute branch creation script: %w", err)
	}

	// Verify user creation was successful
	if !strings.Contains(output, "USER_CREATION_SUCCESS=true") {
		s.logger.Error().Str("output", output).Msg("Branch creation script did not report success")

		return nil, fmt.Errorf("branch creation script failed")
	}

	// Parse port number from branch creation script output
	port, err := s.parseBranchPortFromOutput(output)
	if err != nil {
		s.logger.Error().Err(err).Str("output", output).Msg("Failed to parse port from script output")

		return nil, fmt.Errorf("failed to parse port from script output: %w", err)
	}

	// Verify the port matches the forced port
	if port != forcePort {
		s.logger.Error().
			Int("expected_port", forcePort).
			Int("actual_port", port).
			Msg("Port mismatch during forced branch creation")

		return nil, fmt.Errorf("port mismatch: expected port %d, got %d", forcePort, port)
	}

	// Create branch record in database (only after successful creation)
	branch := models.Branch{
		Name:        params.BranchName,
		RestoreID:   restore.ID,
		CreatedByID: params.CreatedByID,
		User:        user,
		Password:    password,
		Port:        port,
	}

	if err := s.db.Create(&branch).Error; err != nil {
		s.logger.Error().Err(err).Msg("Failed to create branch record")
		return nil, fmt.Errorf("failed to create branch record: %w", err)
	}

	s.logger.Info().
		Str("branch_id", branch.ID).
		Str("branch_name", params.BranchName).
		Int("port", port).
		Msg("Branch created successfully with forced port")

	return &branch, nil
}

// DeleteBranchParams contains parameters for branch deletion
type DeleteBranchParams struct {
	BranchName string
}

// DeleteBranch deletes a branch synchronously
func (s *Service) DeleteBranch(ctx context.Context, params DeleteBranchParams) error {
	s.logger.Info().
		Str("branch_name", params.BranchName).
		Msg("Starting branch deletion")

	// Load branch from database
	var branch models.Branch
	err := s.db.Where("name = ?", params.BranchName).First(&branch).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			s.logger.Warn().
				Str("branch_name", params.BranchName).
				Msg("Branch not found in database - may have been already deleted")
			return fmt.Errorf("branch not found: %s", params.BranchName)
		}
		s.logger.Error().Err(err).Str("branch_name", params.BranchName).Msg("Failed to load branch")
		return fmt.Errorf("failed to load branch: %w", err)
	}

	// Load config (singleton) to get dataset name
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("configuration not found")
		}
		s.logger.Error().Err(err).Msg("Failed to load config")
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Render deletion script
	// Use PostgreSQL version cluster dataset (e.g., tank/pg16)
	datasetName := fmt.Sprintf("tank/pg%s", config.PostgresVersion)
	scriptParams := deleteBranchScriptParams{
		BranchName:  params.BranchName,
		DatasetName: datasetName,
	}

	tmpl, err := template.New("delete-branch").Parse(destroyBranchScript)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to parse script template")
		return fmt.Errorf("failed to parse script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, scriptParams); err != nil {
		s.logger.Error().Err(err).Msg("Failed to execute script template")
		return fmt.Errorf("failed to execute script template: %w", err)
	}

	script := buf.String()

	// Execute deletion script locally (best effort - log errors but continue)
	s.logger.Info().
		Str("branch_name", params.BranchName).
		Msg("Executing deletion script locally")

	cmd := exec.CommandContext(ctx, "bash", "-c", script)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		s.logger.Error().
			Err(err).
			Str("branch_name", params.BranchName).
			Str("output", output).
			Msg("Failed to execute deletion script locally")
		return fmt.Errorf("failed to execute deletion script: %w", err)
	}

	// Verify script reported success
	if !strings.Contains(output, "BRANCH_DELETION_SUCCESS=true") {
		s.logger.Error().
			Str("output", output).
			Str("branch_name", params.BranchName).
			Msg("Branch deletion script did not report success")
		return fmt.Errorf("branch deletion script failed: script did not report success")
	}

	s.logger.Info().
		Str("branch_name", params.BranchName).
		Msg("Branch resources cleaned up successfully")

	// Delete branch from database (this is the critical part)
	if err := s.db.Delete(&branch).Error; err != nil {
		s.logger.Error().
			Err(err).
			Str("branch_id", branch.ID).
			Str("branch_name", params.BranchName).
			Msg("Failed to delete branch from database")
		return fmt.Errorf("failed to delete branch from database: %w", err)
	}

	s.logger.Info().
		Str("branch_id", branch.ID).
		Str("branch_name", params.BranchName).
		Msg("Branch deleted successfully")

	return nil
}

// killRestoreProcess kills an active restore process if it's running
func (s *Service) killRestoreProcess(ctx context.Context, restoreName string) {
	pidFile := fmt.Sprintf("/var/log/branchd/restore-%s.pid", restoreName)
	logFile := fmt.Sprintf("/var/log/branchd/restore-%s.log", restoreName)

	// Check if PID file exists and process is running
	checkCmd := fmt.Sprintf(`
		if [ -f %s ]; then
			pid=$(cat %s)
			if kill -0 $pid 2>/dev/null; then
				echo "running:$pid"
			else
				echo "stopped"
			fi
		else
			echo "not_found"
		fi
	`, pidFile, pidFile)

	cmd := exec.CommandContext(ctx, "bash", "-c", checkCmd)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Warn().
			Err(err).
			Str("restore_name", restoreName).
			Msg("Failed to check restore process status")
	}

	output := strings.TrimSpace(string(outputBytes))

	// If process is running, kill it
	if strings.HasPrefix(output, "running:") {
		pid := strings.TrimPrefix(output, "running:")

		// Kill the process (SIGTERM first, then SIGKILL if needed)
		killCmd := fmt.Sprintf(`
			pid=%s
			if kill -0 $pid 2>/dev/null; then
				kill -TERM $pid 2>/dev/null || true
				sleep 1
				if kill -0 $pid 2>/dev/null; then
					kill -KILL $pid 2>/dev/null || true
				fi
			fi
		`, pid)

		killExecCmd := exec.CommandContext(ctx, "bash", "-c", killCmd)
		if killOutput, killErr := killExecCmd.CombinedOutput(); killErr != nil {
			s.logger.Warn().
				Err(killErr).
				Str("output", string(killOutput)).
				Str("pid", pid).
				Msg("Failed to kill restore process")
		} else {
			s.logger.Info().
				Str("restore_name", restoreName).
				Str("pid", pid).
				Msg("Restore process killed successfully")
		}
	}

	// Clean up PID and log files
	cleanupCmd := fmt.Sprintf("rm -f %s %s", pidFile, logFile)
	cleanupExecCmd := exec.CommandContext(ctx, "bash", "-c", cleanupCmd)
	if cleanupOutput, cleanupErr := cleanupExecCmd.CombinedOutput(); cleanupErr != nil {
		s.logger.Warn().
			Err(cleanupErr).
			Str("output", string(cleanupOutput)).
			Msg("Failed to clean up restore files")
	} else {
		s.logger.Debug().
			Str("restore_name", restoreName).
			Msg("Restore PID and log files cleaned up")
	}
}

// DeleteRestore deletes a restore from the main cluster
func (s *Service) DeleteRestore(ctx context.Context, restore *models.Restore) error {
	// Load config to get PostgreSQL version
	var config models.Config
	if err := s.db.First(&config).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("configuration not found")
		}
		s.logger.Error().Err(err).Msg("Failed to load config")
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Map PostgreSQL version to port
	portMap := map[string]int{
		"14": 5414,
		"15": 5415,
		"16": 5416,
		"17": 5417,
	}

	port, ok := portMap[config.PostgresVersion]
	if !ok {
		return fmt.Errorf("unsupported PostgreSQL version: %s", config.PostgresVersion)
	}

	// Kill any active restore process before dropping the database
	s.killRestoreProcess(ctx, restore.Name)

	// Terminate all active connections to the restore database
	s.logger.Info().
		Str("restore_name", restore.Name).
		Msg("Terminating active connections to restore database")

	terminateCmd := fmt.Sprintf(
		"sudo -u postgres psql -p %d -c \"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()\"",
		port, restore.Name)
	terminateExecCmd := exec.CommandContext(ctx, "bash", "-c", terminateCmd)
	terminateOutput, terminateErr := terminateExecCmd.CombinedOutput()
	if terminateErr != nil {
		s.logger.Warn().
			Err(terminateErr).
			Str("restore_name", restore.Name).
			Str("output", string(terminateOutput)).
			Msg("Failed to terminate connections (will try to drop anyway)")
	} else {
		s.logger.Info().
			Str("restore_name", restore.Name).
			Str("output", string(terminateOutput)).
			Msg("Terminated active connections to restore database")
	}

	// Drop restore database from PostgreSQL cluster
	dropCmd := fmt.Sprintf("sudo -u postgres psql -p %d -c 'DROP DATABASE IF EXISTS \"%s\"'", port, restore.Name)
	cmd := exec.CommandContext(ctx, "bash", "-c", dropCmd)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		s.logger.Error().
			Err(err).
			Str("restore_name", restore.Name).
			Str("output", output).
			Msg("Failed to drop restore from PostgreSQL")
		return fmt.Errorf("failed to drop restore from PostgreSQL: %w", err)
	}

	s.logger.Info().
		Str("restore_name", restore.Name).
		Int("port", port).
		Msg("Restore dropped from PostgreSQL cluster")

	// Note: Restores are logical databases in the main cluster, not separate ZFS datasets.
	// No ZFS cleanup needed - branches are cloned from the main pg version dataset (e.g., tank/pg16).

	// Delete restore record from SQLite
	if err := s.db.Delete(restore).Error; err != nil {
		s.logger.Error().
			Err(err).
			Str("restore_id", restore.ID).
			Msg("Failed to delete restore record")
		return fmt.Errorf("failed to delete restore record: %w", err)
	}

	s.logger.Info().
		Str("restore_id", restore.ID).
		Str("restore_name", restore.Name).
		Msg("Restore deleted successfully")

	return nil
}
