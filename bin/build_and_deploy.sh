#!/bin/bash
set -euo pipefail

# Configuration via environment variables
VM_IP="${VM_IP:-}"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_rsa}"
SSH_USER="${SSH_USER:-ubuntu}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Validate required environment variables
if [ -z "$VM_IP" ]; then
    log_error "VM_IP environment variable is required"
    echo "Usage: VM_IP=1.2.3.4 SSH_KEY=/path/to/key $0"
    exit 1
fi

if [ ! -f "$SSH_KEY" ]; then
    log_error "SSH key not found at: $SSH_KEY"
    exit 1
fi

# Get project root (script is in bin/ directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

log_info "Building Branchd binaries and web UI..."

# Build server binary (ARM64 for t4g instance)
log_info "Building server binary (ARM64)..."
cd "$PROJECT_ROOT"
GOOS=linux GOARCH=arm64 go build -o /tmp/branchd-server ./cmd/server
if [ $? -ne 0 ]; then
    log_error "Failed to build server binary"
    exit 1
fi

# Build worker binary (ARM64 for t4g instance)
log_info "Building worker binary (ARM64)..."
GOOS=linux GOARCH=arm64 go build -o /tmp/branchd-worker ./cmd/worker
if [ $? -ne 0 ]; then
    log_error "Failed to build worker binary"
    exit 1
fi

# Build web UI
log_info "Building web UI..."
cd "$PROJECT_ROOT/web"
bun run build
if [ $? -ne 0 ]; then
    log_error "Failed to build web UI"
    exit 1
fi

# Create bundle directory structure
BUNDLE_DIR="/tmp/branchd-bundle"
log_info "Creating bundle directory structure..."
rm -rf "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR/web"

# Copy binaries to bundle
log_info "Copying binaries to bundle..."
cp /tmp/branchd-server "$BUNDLE_DIR/server"
cp /tmp/branchd-worker "$BUNDLE_DIR/worker"

# Copy web UI to bundle
log_info "Copying web UI to bundle..."
cp -r "$PROJECT_ROOT/web/dist/"* "$BUNDLE_DIR/web/"

# Upload bundle to VM
log_info "Uploading bundle to VM ($VM_IP)..."
ssh -i "$SSH_KEY" \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -o ConnectTimeout=10 \
    "$SSH_USER@$VM_IP" \
    "mkdir -p /tmp/branchd-bundle"

scp -i "$SSH_KEY" \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -o ConnectTimeout=10 \
    -r "$BUNDLE_DIR/." \
    "$SSH_USER@$VM_IP:/tmp/branchd-bundle/"

if [ $? -ne 0 ]; then
    log_error "Failed to upload bundle to VM"
    exit 1
fi

# Helper function to run SSH commands
ssh_exec() {
    ssh -i "$SSH_KEY" \
        -o StrictHostKeyChecking=no \
        -o UserKnownHostsFile=/dev/null \
        -o LogLevel=ERROR \
        -o ConnectTimeout=10 \
        "$SSH_USER@$VM_IP" \
        "$1"
}

# Install binaries on VM
log_info "Installing binaries on VM..."
ssh_exec "sudo install -m 755 /tmp/branchd-bundle/server /usr/local/bin/branchd-server"
ssh_exec "sudo install -m 755 /tmp/branchd-bundle/worker /usr/local/bin/branchd-worker"

# Install web UI on VM
log_info "Installing web UI on VM..."
ssh_exec "sudo rm -rf /var/www/branchd/*"
ssh_exec "sudo cp -r /tmp/branchd-bundle/web/* /var/www/branchd/"
ssh_exec "sudo chown -R caddy:caddy /var/www/branchd"

# Create data directory if it doesn't exist
log_info "Creating data directory..."
ssh_exec "sudo mkdir -p /data"
ssh_exec "sudo chmod 755 /data"

# Restart Caddy (web UI is now deployed)
log_info "Restarting Caddy web server..."
ssh_exec "sudo systemctl restart caddy"

# Restart Branchd services
log_info "Restarting Branchd services..."
ssh_exec "sudo systemctl daemon-reload"
ssh_exec "sudo systemctl restart branchd-server branchd-worker"

# Wait a moment for services to start
log_info "Waiting for services to start..."
sleep 5

# Verify services are running
log_info "Verifying services..."
SERVER_STATUS=$(ssh_exec "systemctl is-active branchd-server" || echo "failed")
WORKER_STATUS=$(ssh_exec "systemctl is-active branchd-worker" || echo "failed")

if [ "$SERVER_STATUS" = "active" ] && [ "$WORKER_STATUS" = "active" ]; then
    log_info "✓ Branchd deployment complete!"
    log_info "  Server status: $SERVER_STATUS"
    log_info "  Worker status: $WORKER_STATUS"
    log_info "  Web UI: https://$VM_IP"
else
    log_error "Service verification failed"
    log_error "  Server status: $SERVER_STATUS"
    log_error "  Worker status: $WORKER_STATUS"
    exit 1
fi

# Cleanup
log_info "Cleaning up temporary files..."
rm -rf "$BUNDLE_DIR"
rm -f /tmp/branchd-server /tmp/branchd-worker

log_info "Done!"
