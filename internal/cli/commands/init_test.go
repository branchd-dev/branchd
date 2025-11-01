package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/branchd-dev/branchd/internal/cli/config"
)

// TestInitCommand_NewConfig tests creating a brand new config file
func TestInitCommand_NewConfig(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Change to temp directory
	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Run init command
	err = runInitWithOptions([]string{"192.168.1.100"}, &initOptions{skipBrowser: true})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Verify branchd.json was created
	configPath := filepath.Join(tempDir, "branchd.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("branchd.json was not created")
	}

	// Verify config contents
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load created config: %v", err)
	}

	if len(cfg.Servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(cfg.Servers))
	}

	if cfg.Servers[0].IP != "192.168.1.100" {
		t.Errorf("expected IP '192.168.1.100', got '%s'", cfg.Servers[0].IP)
	}

	// First server should have alias "server-1"
	if cfg.Servers[0].Alias != "server-1" {
		t.Errorf("expected alias 'server-1', got '%s'", cfg.Servers[0].Alias)
	}
}

// TestInitCommand_FirstServerGetsServer1Alias tests that first server is named "server-1"
func TestInitCommand_FirstServerGetsServer1Alias(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Run init
	err = runInitWithOptions([]string{"10.0.0.1"}, &initOptions{skipBrowser: true})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Load and verify
	cfg, err := config.Load(filepath.Join(tempDir, "branchd.json"))
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Servers[0].Alias != "server-1" {
		t.Errorf("first server should have alias 'server-1', got '%s'", cfg.Servers[0].Alias)
	}
}

// TestInitCommand_AddSecondServer tests adding a second server to existing config
func TestInitCommand_AddSecondServer(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Create initial config with one server
	initialCfg := &config.Config{
		Servers: []config.Server{
			{IP: "192.168.1.100", Alias: "server-1"},
		},
	}
	configPath := filepath.Join(tempDir, "branchd.json")
	if err := config.Save(configPath, initialCfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	// Add second server
	err = runInitWithOptions([]string{"192.168.1.101"}, &initOptions{skipBrowser: true})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Verify both servers exist
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(cfg.Servers))
	}

	// Verify first server unchanged
	if cfg.Servers[0].IP != "192.168.1.100" || cfg.Servers[0].Alias != "server-1" {
		t.Error("first server was modified")
	}

	// Verify second server
	if cfg.Servers[1].IP != "192.168.1.101" {
		t.Errorf("expected second server IP '192.168.1.101', got '%s'", cfg.Servers[1].IP)
	}

	// Second server should have alias "server-2"
	if cfg.Servers[1].Alias != "server-2" {
		t.Errorf("expected second server alias 'server-2', got '%s'", cfg.Servers[1].Alias)
	}
}

// TestInitCommand_DuplicateServer tests that duplicate IPs are detected
func TestInitCommand_DuplicateServer(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Create initial config
	initialCfg := &config.Config{
		Servers: []config.Server{
			{IP: "192.168.1.100", Alias: "server-1"},
		},
	}
	configPath := filepath.Join(tempDir, "branchd.json")
	if err := config.Save(configPath, initialCfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	// Try to add same server again
	err = runInitWithOptions([]string{"192.168.1.100"}, &initOptions{skipBrowser: true})

	// Should not error, but should not add duplicate
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Verify only one server exists
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Servers) != 1 {
		t.Errorf("expected 1 server (no duplicate), got %d", len(cfg.Servers))
	}
}

// TestInitCommand_MultipleServers tests adding multiple servers and alias naming
func TestInitCommand_MultipleServers(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Add servers one by one
	servers := []struct {
		ip            string
		expectedAlias string
	}{
		{"192.168.1.100", "server-1"},
		{"192.168.1.101", "server-2"},
		{"192.168.1.102", "server-3"},
		{"192.168.1.103", "server-4"},
	}

	for i, srv := range servers {
		err := runInitWithOptions([]string{srv.ip}, &initOptions{skipBrowser: true})
		if err != nil {
			t.Fatalf("init command failed for server %d: %v", i+1, err)
		}
	}

	// Verify all servers
	configPath := filepath.Join(tempDir, "branchd.json")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Servers) != 4 {
		t.Errorf("expected 4 servers, got %d", len(cfg.Servers))
	}

	for i, expected := range servers {
		if cfg.Servers[i].IP != expected.ip {
			t.Errorf("server %d: expected IP '%s', got '%s'", i, expected.ip, cfg.Servers[i].IP)
		}
		if cfg.Servers[i].Alias != expected.expectedAlias {
			t.Errorf("server %d: expected alias '%s', got '%s'", i, expected.expectedAlias, cfg.Servers[i].Alias)
		}
	}
}

// TestInitCommand_MissingArgument tests that init requires an IP address
func TestInitCommand_MissingArgument(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Run init without IP address
	cmd := NewInitCmd()
	cmd.SetArgs([]string{}) // No arguments

	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no IP address provided, but got nil")
	}

	// Should contain "accepts 1 arg(s)" or similar
	if err.Error()[:7] != "accepts" {
		t.Logf("got error: %v", err)
	}
}

// TestInitCommand_ConfigFileFormat tests that config file is properly formatted JSON
func TestInitCommand_ConfigFileFormat(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Run init
	err = runInitWithOptions([]string{"192.168.1.100"}, &initOptions{skipBrowser: true})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Read file and verify it's valid JSON
	configPath := filepath.Join(tempDir, "branchd.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	// Parse as JSON
	var parsedConfig config.Config
	if err := json.Unmarshal(data, &parsedConfig); err != nil {
		t.Fatalf("config file is not valid JSON: %v", err)
	}

	// Verify structure
	if len(parsedConfig.Servers) != 1 {
		t.Errorf("expected 1 server in parsed config, got %d", len(parsedConfig.Servers))
	}
}

// TestInitCommand_PreservesExistingConfig tests that existing servers aren't lost
func TestInitCommand_PreservesExistingConfig(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "branchd-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	originalDir, _ := os.Getwd()
	os.Chdir(tempDir)
	defer os.Chdir(originalDir)

	// Create config with custom server data
	initialCfg := &config.Config{
		Servers: []config.Server{
			{IP: "10.0.0.1", Alias: "custom-production"},
			{IP: "10.0.0.2", Alias: "custom-staging"},
		},
	}
	configPath := filepath.Join(tempDir, "branchd.json")
	if err := config.Save(configPath, initialCfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	// Add a new server
	err = runInitWithOptions([]string{"10.0.0.3"}, &initOptions{skipBrowser: true})
	if err != nil {
		t.Fatalf("init command failed: %v", err)
	}

	// Verify all servers preserved
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Servers) != 3 {
		t.Errorf("expected 3 servers, got %d", len(cfg.Servers))
	}

	// Verify existing servers unchanged
	if cfg.Servers[0].IP != "10.0.0.1" || cfg.Servers[0].Alias != "custom-production" {
		t.Error("first server was modified")
	}
	if cfg.Servers[1].IP != "10.0.0.2" || cfg.Servers[1].Alias != "custom-staging" {
		t.Error("second server was modified")
	}

	// New server should be third with auto-generated alias
	if cfg.Servers[2].IP != "10.0.0.3" {
		t.Errorf("expected third server IP '10.0.0.3', got '%s'", cfg.Servers[2].IP)
	}
	if cfg.Servers[2].Alias != "server-3" {
		t.Errorf("expected third server alias 'server-3', got '%s'", cfg.Servers[2].Alias)
	}
}
