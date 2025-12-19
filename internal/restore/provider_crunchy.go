package restore

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
	"github.com/branchd-dev/branchd/internal/providers"
)

//go:embed crunchy_bridge_restore.sh
var crunchyBridgeRestoreScript string

type crunchyBridgeRestoreParams struct {
	PgVersion          string
	PgPort             int
	RestoreName        string // Name of the restore (e.g., restore_20251211000011) - used for logs, ZFS, service
	TargetDatabaseName string // Actual database name in PostgreSQL
	DataDir            string
	PgBackRestConfPath string
	StanzaName         string
}

// CrunchyBridgeProvider implements restore from Crunchy Bridge backups via pgBackRest
type CrunchyBridgeProvider struct {
	logger zerolog.Logger
}

// NewCrunchyBridgeProvider creates a new Crunchy Bridge restore provider
func NewCrunchyBridgeProvider(logger zerolog.Logger) *CrunchyBridgeProvider {
	return &CrunchyBridgeProvider{
		logger: logger,
	}
}

// GetProviderType returns the provider type identifier
func (p *CrunchyBridgeProvider) GetProviderType() string {
	return string(ProviderTypeCrunchyBridge)
}

// ValidateConfig validates that Crunchy Bridge is properly configured
func (p *CrunchyBridgeProvider) ValidateConfig(config *models.Config) error {
	if config.CrunchyBridgeAPIKey == "" {
		return fmt.Errorf("Crunchy Bridge API key is required")
	}
	if config.CrunchyBridgeClusterName == "" {
		return fmt.Errorf("Crunchy Bridge cluster name is required")
	}
	if config.CrunchyBridgeDatabaseName == "" {
		return fmt.Errorf("Crunchy Bridge database name is required")
	}
	if config.PostgresVersion == "" {
		return fmt.Errorf("PostgreSQL version is required")
	}
	return nil
}

// StartRestore starts the restore process from Crunchy Bridge using pgBackRest
func (p *CrunchyBridgeProvider) StartRestore(ctx context.Context, params ProviderParams) error {
	p.logger.Info().
		Str("restore_id", params.Restore.ID).
		Str("restore_name", params.Restore.Name).
		Str("cluster_name", params.Config.CrunchyBridgeClusterName).
		Int("port", params.Port).
		Msg("Starting Crunchy Bridge restore via pgBackRest")

	// Create Crunchy Bridge client
	client := providers.NewCrunchyBridgeClient(params.Config.CrunchyBridgeAPIKey)

	// Look up cluster by name
	p.logger.Debug().
		Str("cluster_name", params.Config.CrunchyBridgeClusterName).
		Msg("Looking up Crunchy Bridge cluster")

	cluster, err := client.FindClusterByName(params.Config.CrunchyBridgeClusterName)
	if err != nil {
		return fmt.Errorf("failed to find cluster '%s': %w", params.Config.CrunchyBridgeClusterName, err)
	}

	p.logger.Info().
		Str("cluster_id", cluster.ID).
		Str("cluster_name", cluster.Name).
		Int("major_version", cluster.MajorVersion).
		Str("state", cluster.State).
		Msg("Found Crunchy Bridge cluster")

	// Verify cluster is in a usable state
	if cluster.State != "ready" {
		return fmt.Errorf("cluster '%s' is not ready (state: %s)", cluster.Name, cluster.State)
	}

	// Create backup token for accessing backups
	p.logger.Debug().Msg("Creating backup token")
	backupToken, err := client.CreateBackupToken(cluster.ID)
	if err != nil {
		return fmt.Errorf("failed to create backup token: %w", err)
	}

	p.logger.Info().
		Str("repo_type", backupToken.Type).
		Str("repo_path", backupToken.RepoPath).
		Str("stanza", backupToken.Stanza).
		Msg("Backup token created successfully")

	// Calculate paths
	dataDir := fmt.Sprintf("%s/data", params.RestoreDataPath)
	// Write pgBackRest config to /tmp because ZFS mount will overwrite the restore directory
	pgbackrestConfPath := fmt.Sprintf("/tmp/pgbackrest_%s.conf", params.Restore.Name)

	// Generate pgBackRest configuration
	pgbackrestConf := backupToken.GeneratePgBackRestConfig(backupToken.Stanza, dataDir)

	// Write pgBackRest config to /tmp (will be cleaned up by restore script)
	// Note: This contains credentials, so we secure it and clean it up after restore
	// Use 0644 so postgres user can read it for pgBackRest restore
	if err := os.WriteFile(pgbackrestConfPath, []byte(pgbackrestConf), 0644); err != nil {
		return fmt.Errorf("failed to write pgBackRest config: %w", err)
	}

	p.logger.Debug().
		Str("config_path", pgbackrestConfPath).
		Msg("pgBackRest configuration written")

	// Render restore script
	scriptParams := crunchyBridgeRestoreParams{
		PgVersion:          params.Config.PostgresVersion,
		PgPort:             params.Port,
		RestoreName:        params.Restore.Name,
		TargetDatabaseName: params.Config.CrunchyBridgeDatabaseName,
		DataDir:            dataDir,
		PgBackRestConfPath: pgbackrestConfPath,
		StanzaName:         backupToken.Stanza,
	}

	script, err := p.renderScript(scriptParams)
	if err != nil {
		return fmt.Errorf("failed to render Crunchy Bridge restore script: %w", err)
	}

	// Start the restore script in background
	logFile := params.ProcessManager.GetLogFilePath(params.Restore.Name)
	pidFile := params.ProcessManager.GetPIDFilePath(params.Restore.Name)

	// Write script to a temporary file
	scriptPath := fmt.Sprintf("/tmp/branchd_restore_cb_%s.sh", params.Restore.Name)
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
		Msg("Crunchy Bridge restore script started successfully")

	return nil
}

// renderScript renders the bash script template with parameters
func (p *CrunchyBridgeProvider) renderScript(params crunchyBridgeRestoreParams) (string, error) {
	tmpl, err := template.New("crunchy-bridge-restore").Parse(crunchyBridgeRestoreScript)
	if err != nil {
		return "", fmt.Errorf("failed to parse script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute script template: %w", err)
	}

	return buf.String(), nil
}
