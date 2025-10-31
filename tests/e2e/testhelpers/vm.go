package testhelpers

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// VM represents a test EC2 instance with helper methods
type VM struct {
	PublicIP        string
	InstanceID      string
	APIURL          string
	SSHKeyPath      string
	PostgresVersion string
	JWTToken        string // Set after setup/login
}

// GetOrCreateVM gets or creates a persistent test VM via Terraform
func GetOrCreateVM(t *testing.T, postgresVersion string) *VM {
	t.Helper()

	t.Logf("Getting or creating test VM (PostgreSQL %s)...", postgresVersion)

	// Get project root (go up from tests/e2e)
	projectRoot := "../.."

	// Run terraform apply (idempotent)
	cmd := exec.Command("terraform", "apply", "-auto-approve",
		"-var", fmt.Sprintf("postgres_version=%s", postgresVersion),
	)
	cmd.Dir = filepath.Join(projectRoot, "tests/terraform")

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Terraform apply failed: %s", string(output))

	// Parse terraform output
	outputs := getTerraformOutputs(t, projectRoot)

	vm := &VM{
		PublicIP:        outputs["public_ip"],
		InstanceID:      outputs["instance_id"],
		APIURL:          outputs["api_url"],
		SSHKeyPath:      getTerraformVariable(t, projectRoot, "ssh_private_key_path"),
		PostgresVersion: postgresVersion,
	}

	t.Logf("VM ready: %s (instance: %s)", vm.PublicIP, vm.InstanceID)

	// Note: API server won't be ready until BuildAndDeploy is called
	// (that method handles building binaries, deploying, and waiting for API)

	return vm
}

// ResetState cleans up test state using the API (no manual ZFS manipulation)
func (vm *VM) ResetState(t *testing.T) {
	t.Helper()

	t.Log("Resetting test state...")

	// Stop Branchd services and wait for them to fully stop
	vm.SSH(t, "sudo systemctl stop branchd-server branchd-worker")
	vm.SSH(t, "sleep 2") // Wait for processes to fully stop and release file handles

	// Clean up SQLite database and WAL files (will recreate tables on restart)
	vm.SSH(t, "sudo rm -f /data/branchd.sqlite /data/branchd.sqlite-shm /data/branchd.sqlite-wal")

	// Clean up Redis (task queue)
	vm.SSH(t, "redis-cli FLUSHALL >/dev/null 2>&1")

	// Restart services
	vm.SSH(t, "sudo systemctl start branchd-server branchd-worker")
	vm.SSH(t, "sleep 2") // Wait for services to initialize

	// Wait for API to be ready again
	vm.waitForAPI(t)

	t.Log("State reset complete")
}

// SSH executes a command on the VM via SSH
func (vm *VM) SSH(t *testing.T, command string) string {
	t.Helper()

	cmd := exec.Command("ssh",
		"-i", vm.SSHKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-o", "ConnectTimeout=10",
		fmt.Sprintf("ubuntu@%s", vm.PublicIP),
		command,
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "SSH command failed: %s\nOutput: %s", command, string(output))

	return strings.TrimSpace(string(output))
}

// APICall makes an HTTP request to the Branchd API
func (vm *VM) APICall(t *testing.T, method, path string, body interface{}) map[string]interface{} {
	t.Helper()

	url := vm.APIURL + path

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err, "Failed to marshal request body")
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err, "Failed to create request")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add JWT token if available
	if vm.JWTToken != "" {
		req.Header.Set("Authorization", "Bearer "+vm.JWTToken)
	}

	// Create HTTP client that skips SSL verification (self-signed certs)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	require.NoError(t, err, "Request failed: %s %s", method, path)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")

	require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
		"API call failed: %s %s\nStatus: %d\nBody: %s",
		method, path, resp.StatusCode, string(respBody))

	var result map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err, "Failed to unmarshal response: %s", string(respBody))

	return result
}

// APICallList makes an HTTP request expecting a list response
func (vm *VM) APICallList(t *testing.T, method, path string, body interface{}) []map[string]interface{} {
	t.Helper()

	url := vm.APIURL + path

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		require.NoError(t, err, "Failed to marshal request body")
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err, "Failed to create request")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if vm.JWTToken != "" {
		req.Header.Set("Authorization", "Bearer "+vm.JWTToken)
	}

	// Create HTTP client that skips SSL verification (self-signed certs)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	require.NoError(t, err, "Request failed: %s %s", method, path)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "Failed to read response body")

	require.True(t, resp.StatusCode >= 200 && resp.StatusCode < 300,
		"API call failed: %s %s\nStatus: %d\nBody: %s",
		method, path, resp.StatusCode, string(respBody))

	var result []map[string]interface{}
	err = json.Unmarshal(respBody, &result)
	require.NoError(t, err, "Failed to unmarshal response: %s", string(respBody))

	return result
}

// WaitForCondition polls until a condition is met or timeout
func (vm *VM) WaitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()

	start := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if condition() {
			return
		}

		if time.Since(start) > timeout {
			require.FailNow(t, "Timeout waiting for condition")
		}

		<-ticker.C
	}
}

// waitForAPI waits for the API server to be ready
func (vm *VM) waitForAPI(t *testing.T) {
	t.Helper()

	t.Log("Waiting for API server to be ready...")

	// Create HTTP client that skips SSL verification (self-signed certs)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	vm.WaitForCondition(t, 20*time.Second, func() bool {
		resp, err := client.Get(vm.APIURL + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == 200
	})

	t.Log("API server ready")
}

// getTerraformOutputs retrieves all terraform outputs as a map
func getTerraformOutputs(t *testing.T, projectRoot string) map[string]string {
	t.Helper()

	cmd := exec.Command("terraform", "output", "-json")
	cmd.Dir = filepath.Join(projectRoot, "tests/terraform")

	output, err := cmd.Output()
	require.NoError(t, err, "Failed to get terraform outputs")

	var outputs map[string]struct {
		Value string `json:"value"`
	}
	err = json.Unmarshal(output, &outputs)
	require.NoError(t, err, "Failed to parse terraform outputs")

	result := make(map[string]string)
	for key, val := range outputs {
		result[key] = val.Value
	}

	return result
}

// getTerraformVariable retrieves a terraform variable value
func getTerraformVariable(t *testing.T, projectRoot string, varName string) string {
	t.Helper()

	cmd := exec.Command("terraform", "console")
	cmd.Dir = filepath.Join(projectRoot, "tests/terraform")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("var.%s", varName))

	output, err := cmd.Output()
	require.NoError(t, err, "Failed to get terraform variable: %s", varName)

	// Terraform console outputs with quotes, strip them
	return strings.Trim(strings.TrimSpace(string(output)), "\"")
}

// BuildAndDeploy builds the Branchd binaries and web UI, then deploys them to the VM
func (vm *VM) BuildAndDeploy(t *testing.T) {
	t.Helper()

	t.Log("Building Branchd binaries and web UI...")

	// Get project root (go up from tests/e2e)
	projectRoot := "../.."

	// Build server binary (ARM64 for t4g instance)
	t.Log("Building server binary (ARM64)...")
	serverCmd := exec.Command("go", "build", "-o", "/tmp/branchd-server", "./cmd/server")
	serverCmd.Dir = projectRoot
	serverCmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64")
	output, err := serverCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build server binary: %s", string(output))

	// Build worker binary (ARM64 for t4g instance)
	t.Log("Building worker binary (ARM64)...")
	workerCmd := exec.Command("go", "build", "-o", "/tmp/branchd-worker", "./cmd/worker")
	workerCmd.Dir = projectRoot
	workerCmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64")
	output, err = workerCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build worker binary: %s", string(output))

	// Build web UI
	t.Log("Building web UI...")
	webBuildCmd := exec.Command("bun", "run", "build")
	webBuildCmd.Dir = filepath.Join(projectRoot, "web")
	output, err = webBuildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build web UI: %s", string(output))

	// Create bundle directory structure
	bundleDir := "/tmp/branchd-bundle"
	t.Log("Creating bundle directory structure...")
	os.RemoveAll(bundleDir) // Clean up any previous bundle
	require.NoError(t, os.MkdirAll(filepath.Join(bundleDir, "web"), 0755))

	// Copy binaries to bundle
	copyFile(t, "/tmp/branchd-server", filepath.Join(bundleDir, "server"))
	copyFile(t, "/tmp/branchd-worker", filepath.Join(bundleDir, "worker"))

	// Copy web UI to bundle
	webDistDir := filepath.Join(projectRoot, "web/dist")
	copyDir(t, webDistDir, filepath.Join(bundleDir, "web"))

	// Upload bundle to VM
	t.Log("Uploading bundle to VM...")
	vm.UploadDir(t, bundleDir, "/tmp/branchd-bundle")

	// Install binaries on VM
	t.Log("Installing binaries on VM...")
	vm.SSH(t, "sudo install -m 755 /tmp/branchd-bundle/server /usr/local/bin/branchd-server")
	vm.SSH(t, "sudo install -m 755 /tmp/branchd-bundle/worker /usr/local/bin/branchd-worker")

	// Install web UI on VM
	t.Log("Installing web UI on VM...")
	vm.SSH(t, "sudo rm -rf /var/www/branchd/*")
	vm.SSH(t, "sudo cp -r /tmp/branchd-bundle/web/* /var/www/branchd/")
	vm.SSH(t, "sudo chown -R caddy:caddy /var/www/branchd")

	// Create data directory if it doesn't exist
	vm.SSH(t, "sudo mkdir -p /data")
	vm.SSH(t, "sudo chmod 755 /data")

	// Start Caddy (web UI is now deployed)
	t.Log("Starting Caddy web server...")
	vm.SSH(t, "sudo systemctl restart caddy")

	// Start Branchd services
	t.Log("Starting Branchd services...")
	vm.SSH(t, "sudo systemctl daemon-reload")
	vm.SSH(t, "sudo systemctl start branchd-server branchd-worker")

	// Wait a moment for services to start
	time.Sleep(5 * time.Second)

	// Verify services are running
	serverStatus := vm.SSH(t, "systemctl is-active branchd-server")
	workerStatus := vm.SSH(t, "systemctl is-active branchd-worker")
	require.Equal(t, "active", serverStatus, "branchd-server should be active")
	require.Equal(t, "active", workerStatus, "branchd-worker should be active")

	t.Log("Branchd deployment complete")

	// Wait for API to be ready
	vm.waitForAPI(t)
}

// UploadDir uploads a directory to the VM via SCP
func (vm *VM) UploadDir(t *testing.T, localDir, remoteDir string) {
	t.Helper()

	// Create remote directory
	vm.SSH(t, fmt.Sprintf("mkdir -p %s", remoteDir))

	// Use scp with recursive flag to upload directory
	cmd := exec.Command("scp",
		"-i", vm.SSHKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-r",          // Recursive
		localDir+"/.", // Upload contents of directory
		fmt.Sprintf("ubuntu@%s:%s", vm.PublicIP, remoteDir),
	)

	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to upload directory %s: %s", localDir, string(output))
}

// copyFile copies a file from src to dst
func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	input, err := os.ReadFile(src)
	require.NoError(t, err, "Failed to read source file: %s", src)

	err = os.WriteFile(dst, input, 0755)
	require.NoError(t, err, "Failed to write destination file: %s", dst)
}

// copyDir recursively copies a directory
func copyDir(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Copy file
		input, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, input, info.Mode())
	})

	require.NoError(t, err, "Failed to copy directory: %s", src)
}
