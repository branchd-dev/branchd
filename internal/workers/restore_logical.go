package workers

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"text/template"

	"github.com/rs/zerolog"

	"github.com/branchd-dev/branchd/internal/models"
	"github.com/branchd-dev/branchd/internal/pgtuning"
	"github.com/branchd-dev/branchd/internal/restore"
	"github.com/branchd-dev/branchd/internal/sysinfo"
)

//go:embed logical_restore.sh
var logicalRestoreScript string

type logicalRestoreParams struct {
	ConnectionString string
	PgVersion        string
	PgPort           int // Dynamic port for this restore's cluster
	DatabaseName     string
	SchemaOnly       string // "true" or "false" for template
	ParallelJobs     int
	DumpDir          string // Directory for pg_dump output (on EBS zpool)
	DataDir          string // PostgreSQL data directory for initdb

	// PostgreSQL tuning parameters
	TuneSQL  []string // SQL statements to apply tuning
	ResetSQL []string // SQL statements to reset tuning
}

// LogicalRestoreProvider implements logical restore via pg_dump/pg_restore
type LogicalRestoreProvider struct {
	logger zerolog.Logger
}

// NewLogicalRestoreProvider creates a new logical restore provider
func NewLogicalRestoreProvider(logger zerolog.Logger) *LogicalRestoreProvider {
	return &LogicalRestoreProvider{
		logger: logger,
	}
}

// GetProviderType returns the provider type identifier
func (p *LogicalRestoreProvider) GetProviderType() string {
	return "logical"
}

// ValidateConfig validates that logical restore is properly configured
func (p *LogicalRestoreProvider) ValidateConfig(config *models.Config) error {
	if config.ConnectionString == "" {
		return fmt.Errorf("connection string is required for logical restore")
	}
	if config.PostgresVersion == "" {
		return fmt.Errorf("PostgreSQL version is required")
	}
	return nil
}

// StartRestore starts the logical restore process using pg_dump/pg_restore
func (p *LogicalRestoreProvider) StartRestore(ctx context.Context, params RestoreParams) error {
	p.logger.Info().
		Str("restore_id", params.Restore.ID).
		Str("restore_name", params.Restore.Name).
		Int("port", params.Port).
		Msg("Starting logical restore via pg_dump/pg_restore")

	// Validate inputs using orchestrator
	orchestrator := restore.NewOrchestrator(p.logger)
	if err := orchestrator.ValidateInputs(
		params.Config.ConnectionString,
		params.Config.PostgresVersion,
		params.Port,
		params.Restore.Name,
	); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Detect system resources and calculate optimal settings
	resources, err := sysinfo.GetResources()
	if err != nil {
		p.logger.Warn().Err(err).Msg("Failed to detect system resources, using defaults")
	}

	tuning := pgtuning.CalculateOptimalSettings(resources)

	// Calculate paths for restore cluster
	dataDir := fmt.Sprintf("%s/data", params.RestoreDataPath)        // PostgreSQL data directory
	dumpDir := fmt.Sprintf("%s/dump.pgdump", params.RestoreDataPath) // pg_dump output file

	// Render restore script
	schemaOnlyStr := "false"
	if params.Restore.SchemaOnly {
		schemaOnlyStr = "true"
	}

	scriptParams := logicalRestoreParams{
		ConnectionString: params.Config.ConnectionString,
		PgVersion:        params.Config.PostgresVersion,
		PgPort:           params.Port,
		DatabaseName:     params.Restore.Name,
		SchemaOnly:       schemaOnlyStr,
		ParallelJobs:     tuning.ParallelJobs,
		DumpDir:          dumpDir,
		DataDir:          dataDir,
		TuneSQL:          tuning.GenerateAlterSystemSQL(),
		ResetSQL:         pgtuning.GenerateResetSQL(),
	}

	script, err := p.renderScript(scriptParams)
	if err != nil {
		return fmt.Errorf("failed to render logical restore script: %w", err)
	}

	// Start the restore script in background using nohup
	logFile := orchestrator.GetLogFilePath(params.Restore.Name)
	pidFile := orchestrator.GetPIDFilePath(params.Restore.Name)

	// Write script to a temporary file to avoid shell quoting issues
	scriptPath := fmt.Sprintf("/tmp/branchd_restore_%s.sh", params.Restore.Name)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write restore script: %w", err)
	}

	// Create a wrapper script that runs the restore in background and cleans up the temp file
	wrapperScript := fmt.Sprintf(`
		nohup bash -c 'bash "%s"; rm -f "%s"' > "%s" 2>&1 &
		echo $! > "%s"
	`, scriptPath, scriptPath, logFile, pidFile)

	cmd := exec.CommandContext(ctx, "bash", "-c", wrapperScript)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		p.logger.Error().Err(err).Str("output", output).Msg("Failed to start restore script")
		return fmt.Errorf("restore script execution failed: %w", err)
	}

	p.logger.Info().
		Str("restore_id", params.Restore.ID).
		Str("log_file", logFile).
		Str("pid_file", pidFile).
		Msg("Logical restore script started successfully")

	return nil
}

// renderScript renders the bash script template with parameters
func (p *LogicalRestoreProvider) renderScript(params logicalRestoreParams) (string, error) {
	tmpl, err := template.New("logical-restore").Parse(logicalRestoreScript)
	if err != nil {
		return "", fmt.Errorf("failed to parse script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute script template: %w", err)
	}

	return buf.String(), nil
}
