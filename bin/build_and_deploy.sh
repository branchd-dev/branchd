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

log_info "Building Branchd binaries and web UI in parallel..."

# Create bundle directory structure
BUNDLE_DIR="/tmp/branchd-bundle"
rm -rf "$BUNDLE_DIR"
mkdir -p "$BUNDLE_DIR/web"

# Build all components in parallel
BUILD_FAILED=0

# Server binary (ARM64)
(
    log_info "Building server binary (ARM64)..."
    cd "$PROJECT_ROOT"
    if GOOS=linux GOARCH=arm64 go build -o "$BUNDLE_DIR/server" ./cmd/server 2>&1; then
        log_info "✓ Server binary built"
    else
        log_error "Failed to build server binary"
        exit 1
    fi
) &
SERVER_PID=$!

# Worker binary (ARM64)
(
    log_info "Building worker binary (ARM64)..."
    cd "$PROJECT_ROOT"
    if GOOS=linux GOARCH=arm64 go build -o "$BUNDLE_DIR/worker" ./cmd/worker 2>&1; then
        log_info "✓ Worker binary built"
    else
        log_error "Failed to build worker binary"
        exit 1
    fi
) &
WORKER_PID=$!

# Web UI
(
    log_info "Building web UI..."
    cd "$PROJECT_ROOT/web"
    if bun run build 2>&1; then
        cp -r "$PROJECT_ROOT/web/dist/"* "$BUNDLE_DIR/web/"
        log_info "✓ Web UI built"
    else
        log_error "Failed to build web UI"
        exit 1
    fi
) &
WEB_PID=$!

# Wait for all builds to complete
wait $SERVER_PID || BUILD_FAILED=1
wait $WORKER_PID || BUILD_FAILED=1
wait $WEB_PID || BUILD_FAILED=1

if [ $BUILD_FAILED -eq 1 ]; then
    log_error "One or more builds failed"
    exit 1
fi

log_info "All builds completed successfully"

# Upload and deploy in a single SSH session to minimize RTTs
log_info "Uploading and deploying to VM ($VM_IP)..."

# Use rsync for efficient file transfer (single RTT)
rsync -az --delete \
    -e "ssh -i $SSH_KEY -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10" \
    "$BUNDLE_DIR/" \
    "$SSH_USER@$VM_IP:/tmp/branchd-bundle/"

if [ $? -ne 0 ]; then
    log_error "Failed to upload bundle to VM"
    exit 1
fi

# Execute all remote operations in a single SSH session
log_info "Installing and restarting services..."
REMOTE_OUTPUT=$(ssh -i "$SSH_KEY" \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -o ConnectTimeout=10 \
    "$SSH_USER@$VM_IP" /bin/bash <<'EOF'
set -euo pipefail

# Install binaries
sudo install -m 755 /tmp/branchd-bundle/server /usr/local/bin/branchd-server
sudo install -m 755 /tmp/branchd-bundle/worker /usr/local/bin/branchd-worker

# Install web UI
sudo rm -rf /var/www/branchd/*
sudo cp -r /tmp/branchd-bundle/web/* /var/www/branchd/
sudo chown -R caddy:caddy /var/www/branchd

# Create data directory
sudo mkdir -p /data
sudo chmod 755 /data

# Restart services
sudo systemctl restart caddy
sudo systemctl daemon-reload
sudo systemctl restart branchd-server branchd-worker
sleep 1

# Verify services and output status
SERVER_STATUS=$(systemctl is-active branchd-server || echo "failed")
WORKER_STATUS=$(systemctl is-active branchd-worker || echo "failed")

echo "SERVER_STATUS=$SERVER_STATUS"
echo "WORKER_STATUS=$WORKER_STATUS"

if [ "$SERVER_STATUS" != "active" ] || [ "$WORKER_STATUS" != "active" ]; then
    exit 1
fi
EOF
)

# Check if remote commands succeeded
if [ $? -ne 0 ]; then
    log_error "Deployment failed on VM"
    log_error "$REMOTE_OUTPUT"
    exit 1
fi

# Parse status from output
SERVER_STATUS=$(echo "$REMOTE_OUTPUT" | grep "SERVER_STATUS=" | cut -d= -f2)
WORKER_STATUS=$(echo "$REMOTE_OUTPUT" | grep "WORKER_STATUS=" | cut -d= -f2)

log_info "✓ Branchd deployment complete!"
log_info "  Server status: $SERVER_STATUS"
log_info "  Worker status: $WORKER_STATUS"
log_info "  Web UI: https://$VM_IP"

# Cleanup
log_info "Cleaning up temporary files..."
rm -rf "$BUNDLE_DIR"

log_info "Done!"
