#!/bin/bash
set -euo pipefail

# Parse command line arguments
PG_VERSION=""
for arg in "$@"; do
    case $arg in
        --pg-version=*)
            PG_VERSION="${arg#*=}"
            shift
            ;;
        *)
            echo "Unknown argument: $arg"
            echo "Usage: $0 --pg-version=14|15|16|17"
            exit 1
            ;;
    esac
done

# Validate PostgreSQL version parameter
if [[ -z "$PG_VERSION" ]]; then
    echo "ERROR: --pg-version parameter is required"
    echo "Usage: $0 --pg-version=14|15|16|17"
    exit 1
fi

if [[ ! "$PG_VERSION" =~ ^(14|15|16|17)$ ]]; then
    echo "ERROR: Invalid PostgreSQL version: $PG_VERSION"
    echo "Valid versions: 14, 15, 16, 17"
    exit 1
fi

echo "=== Starting server setup for PostgreSQL ${PG_VERSION} ==="

# Set environment variables for non-interactive installs
export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_MODE=a
export DEBIAN_PRIORITY=critical
export DEBCONF_NONINTERACTIVE_SEEN=true

# Configure debconf to use noninteractive frontend
echo 'debconf debconf/frontend select Noninteractive' | sudo debconf-set-selections
echo 'debconf debconf/priority select critical' | sudo debconf-set-selections

# Set timezone to UTC
echo "Setting timezone to UTC..."
sudo timedatectl set-timezone UTC

# Update package cache
echo "Updating package cache..."
sudo apt-get update

# Add PostgreSQL official repository
echo "Adding PostgreSQL official repository..."
wget --quiet -O - https://www.postgresql.org/media/keys/ACCC4CF8.asc | \
    sudo gpg --dearmor -o /usr/share/keyrings/postgresql-archive-keyring.gpg
# Hardcoded for Ubuntu 24.04 LTS (noble)
echo "deb [signed-by=/usr/share/keyrings/postgresql-archive-keyring.gpg] http://apt.postgresql.org/pub/repos/apt/ noble-pgdg main" | \
    sudo tee /etc/apt/sources.list.d/pgdg.list
sudo apt-get update

# Prevent automatic PostgreSQL cluster creation
echo "Preventing automatic cluster creation..."
sudo mkdir -p /etc/postgresql-common
echo "create_main_cluster = false" | sudo tee /etc/postgresql-common/createcluster.conf

# Disable needrestart prompts
echo "Disabling needrestart service checks..."
sudo mkdir -p /etc/needrestart
sudo tee /etc/needrestart/conf.d/no-prompt.conf > /dev/null << 'EOF'
# Disable needrestart prompts during package installation
$nrconf{restart} = 'a';
$nrconf{kernelhints} = 0;
EOF

# Install essential packages
echo "Installing essential packages..."
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" python3-pip python3-venv wget ca-certificates gnupg sqlite3

# Install security packages
echo "Installing security packages..."
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" fail2ban unattended-upgrades ufw

# Install utilities
echo "Installing utilities..."
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" jq

# Install ZFS utilities
echo "Installing ZFS utilities..."
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" zfsutils-linux

# Install PostgreSQL for specified version
echo "Installing PostgreSQL ${PG_VERSION}..."
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" \
    "postgresql-${PG_VERSION}" \
    "postgresql-client-${PG_VERSION}" \
    "postgresql-contrib-${PG_VERSION}" \
    "postgresql-server-dev-${PG_VERSION}" \
    "postgresql-${PG_VERSION}-pgaudit" \
    "postgresql-${PG_VERSION}-cron" \
    "postgresql-${PG_VERSION}-squeeze" \
    "postgresql-${PG_VERSION}-postgis-3"

echo "Verifying PostgreSQL ${PG_VERSION} installation..."
if [ ! -f "/usr/lib/postgresql/${PG_VERSION}/bin/psql" ] || \
   [ ! -f "/usr/lib/postgresql/${PG_VERSION}/bin/postgres" ]; then
    echo "ERROR: PostgreSQL ${PG_VERSION} was not installed properly!"
    exit 1
fi
PG_INSTALLED_VERSION=$(/usr/lib/postgresql/${PG_VERSION}/bin/psql --version | awk '{print $3}')
echo "✓ PostgreSQL ${PG_VERSION} verified: ${PG_INSTALLED_VERSION}"

# Configure ZFS kernel module for persistent loading
echo "Configuring ZFS kernel module..."
echo "zfs" | sudo tee -a /etc/modules
sudo tee /etc/modules-load.d/zfs.conf > /dev/null << EOF
zfs
EOF

# Load ZFS module now
echo "Loading ZFS kernel module..."
sudo modprobe zfs

# Enable ZFS services
echo "Enabling ZFS services..."
sudo systemctl enable zfs-import-cache
sudo systemctl enable zfs-mount
sudo systemctl enable zfs.target

# Clean up package cache
echo "Cleaning package cache..."
sudo apt-get autoclean
sudo apt-get autoremove -y

# Upgrade system packages
echo "Upgrading system packages..."
sudo apt-get upgrade -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold"

echo "=== ZFS Pool Creation Setup ==="

# Create ZFS pool creation script
echo "Creating ZFS pool creation script..."
cat <<'ZFSEOF' | sudo tee /usr/local/bin/create-tank-pool.sh >/dev/null
#!/bin/bash
set -e

PG_VERSION="PG_VERSION_PLACEHOLDER"
PG_PORT="PG_PORT_PLACEHOLDER"

# Auto-create tank pool on boot if not exists
if zpool status tank >/dev/null 2>&1; then
    echo "Tank pool already exists, skipping creation"
    exit 0
fi

# Auto-detect the correct data volume (non-root EBS volume)
echo "Detecting data volume for ZFS..."
ROOT_DEVICE=$(lsblk -no NAME,MOUNTPOINT | grep -E '/$' | awk '{print "/dev/"$1}' | sed 's/[0-9]*$//')
DATA_DEVICE=""

# Find the first NVMe device that's not the root device and not partitioned
for device in /dev/nvme*n1; do
    if [ "$device" != "$ROOT_DEVICE" ] && [ -b "$device" ]; then
        # Check if device has partitions
        if ! lsblk "$device" | grep -q part; then
            DATA_DEVICE="$device"
            echo "Found data volume: $DATA_DEVICE"
            break
        fi
    fi
done

if [ -z "$DATA_DEVICE" ]; then
    echo "ERROR: No suitable data volume found for ZFS"
    echo "Available devices:"
    lsblk
    exit 1
fi

# Wait for the detected device to be ready
echo "Waiting for data volume $DATA_DEVICE to be ready..."
for i in {1..30}; do
    if [ -b "$DATA_DEVICE" ]; then
        echo "Data volume ready after ${i} seconds"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "ERROR: Data volume $DATA_DEVICE not ready after 30 seconds"
        exit 1
    fi
    sleep 1
done

# Additional safety check - ensure device is ready for I/O
echo "Verifying device readiness..."
if ! timeout 10 dd if="$DATA_DEVICE" of=/dev/null bs=1 count=1 2>/dev/null; then
    echo "ERROR: Device $DATA_DEVICE not ready for I/O"
    exit 1
fi

echo "Creating ZFS tank pool on $DATA_DEVICE..."
if zpool create -o autoexpand=on tank "$DATA_DEVICE"; then
    echo "Setting ZFS properties..."
    zfs set recordsize=8K tank
    zfs set compression=lz4 tank
    zfs set atime=off tank
    zfs set logbias=throughput tank
    zfs set mountpoint=/opt/branchd tank

    echo "Creating ZFS dataset for PostgreSQL ${PG_VERSION}..."
    zfs create tank/pg${PG_VERSION}
    chown postgres:postgres /opt/branchd/pg${PG_VERSION}

    echo "Creating PostgreSQL cluster on ZFS dataset..."
    pg_createcluster --port ${PG_PORT} -d /opt/branchd/pg${PG_VERSION}/main ${PG_VERSION} main

    echo "Adding ZFS dependencies to PostgreSQL service..."
    mkdir -p /etc/systemd/system/postgresql@${PG_VERSION}-main.service.d
    cat <<PGEOF > /etc/systemd/system/postgresql@${PG_VERSION}-main.service.d/zfs-dependency.conf
[Unit]
After=zfs-mount.service
Requires=zfs-mount.service
PGEOF
    systemctl daemon-reload

    echo "Enabling and starting PostgreSQL cluster..."
    systemctl enable postgresql@${PG_VERSION}-main
    systemctl start postgresql@${PG_VERSION}-main

    echo "ZFS tank pool and PostgreSQL cluster created successfully"
else
    echo "ERROR: Failed to create ZFS pool"
    exit 1
fi
ZFSEOF

# Replace placeholders in the script
PG_PORT="54${PG_VERSION}"
sudo sed -i "s/PG_VERSION_PLACEHOLDER/${PG_VERSION}/g" /usr/local/bin/create-tank-pool.sh
sudo sed -i "s/PG_PORT_PLACEHOLDER/${PG_PORT}/g" /usr/local/bin/create-tank-pool.sh
sudo chmod +x /usr/local/bin/create-tank-pool.sh
echo "✓ ZFS pool creation script configured for PostgreSQL ${PG_VERSION}"

# Create systemd service for ZFS pool auto-creation
echo "Creating systemd service for ZFS pool auto-creation..."
cat <<'EOF' | sudo tee /etc/systemd/system/create-tank-pool.service >/dev/null
[Unit]
Description=Create ZFS tank pool on boot
After=multi-user.target
Wants=multi-user.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/create-tank-pool.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

# Enable ZFS pool creation service
echo "Enabling ZFS pool creation service..."
sudo mkdir -p /etc/systemd/system/multi-user.target.wants
sudo ln -sf /etc/systemd/system/create-tank-pool.service /etc/systemd/system/multi-user.target.wants/create-tank-pool.service
echo "✓ ZFS pool creation service enabled"

echo "=== PostgreSQL TLS Configuration ==="

# Create directory for PostgreSQL certificates
echo "Creating certificate directory..."
sudo mkdir -p /etc/postgresql-common/ssl
sudo chmod 755 /etc/postgresql-common/ssl

# Generate self-signed certificate valid for 10 years
echo "Generating self-signed certificate..."
sudo openssl req -new -x509 -days 3650 -nodes \
  -out /etc/postgresql-common/ssl/server.crt \
  -keyout /etc/postgresql-common/ssl/server.key \
  -subj "/CN=branchd-postgres"

# Set proper permissions for PostgreSQL
echo "Setting certificate permissions..."
sudo chmod 644 /etc/postgresql-common/ssl/server.crt
sudo chmod 600 /etc/postgresql-common/ssl/server.key
sudo chown postgres:postgres /etc/postgresql-common/ssl/server.key
sudo chown postgres:postgres /etc/postgresql-common/ssl/server.crt

# Verify certificate was created
if [ ! -f /etc/postgresql-common/ssl/server.crt ] || [ ! -f /etc/postgresql-common/ssl/server.key ]; then
    echo "ERROR: TLS certificate not created!"
    exit 1
fi

echo "Certificate details:"
openssl x509 -in /etc/postgresql-common/ssl/server.crt -noout -subject -dates
echo "✓ PostgreSQL TLS certificates configured"

# Configure system limits for PostgreSQL
echo "Setting PostgreSQL system limits..."
sudo tee /etc/security/limits.d/99-postgres.conf > /dev/null << 'EOF'
# PostgreSQL limits
postgres soft nofile 65536
postgres hard nofile 65536
postgres soft nproc 32768
postgres hard nproc 32768
EOF

echo "=== fail2ban Configuration ==="

# Configure fail2ban base settings
echo "Configuring fail2ban base settings..."
sudo tee /etc/fail2ban/jail.local > /dev/null << 'EOF'
[DEFAULT]
bantime = 1800
findtime = 600
maxretry = 3
backend = systemd

# SSH jail explicitly disabled - SSH access blocked by security group
[sshd]
enabled = false
EOF

# Configure PostgreSQL-specific fail2ban protection
echo "Configuring PostgreSQL fail2ban protection..."
sudo tee /etc/fail2ban/jail.d/postgresql-security.conf > /dev/null << 'EOF'
[postgresql-portscan]
enabled = true
port = 15432:16432
filter = postgresql-portscan
logpath = /var/log/ufw.log
maxretry = 1
bantime = 3600
findtime = 300

[postgresql-auth]
enabled = true
port = 15432:16432
filter = postgresql-auth
logpath = /var/log/postgresql/postgresql-*.log
maxretry = 3
bantime = 3600
findtime = 600
EOF

# Create PostgreSQL port scan filter
echo "Creating PostgreSQL port scan filter..."
sudo tee /etc/fail2ban/filter.d/postgresql-portscan.conf > /dev/null << 'EOF'
[Definition]
failregex = .*\[UFW BLOCK\].*DPT=(1543[2-9]|154[3-9]\d|15[5-9]\d{2}|16[0-3]\d{2}|164[0-2]\d|1643[0-2]).*SRC=<HOST>
ignoreregex =
EOF

# Create PostgreSQL authentication failure filter
echo "Creating PostgreSQL authentication failure filter..."
sudo tee /etc/fail2ban/filter.d/postgresql-auth.conf > /dev/null << 'EOF'
[Definition]
# Detect failed password authentication attempts
failregex = ^.*FATAL:\s+password authentication failed for user.*HOST=<HOST>
            ^.*FATAL:\s+no pg_hba\.conf entry for host "<HOST>"
ignoreregex =
EOF

# Validate fail2ban configuration
echo "Validating fail2ban configuration..."
if ! sudo fail2ban-client -t; then
    echo "ERROR: fail2ban configuration is invalid!"
    exit 1
fi

# Restart fail2ban to apply changes
echo "Restarting fail2ban service..."
sudo systemctl restart fail2ban
sudo systemctl enable fail2ban

# Verify fail2ban is running
if ! sudo systemctl is-active --quiet fail2ban; then
    echo "ERROR: fail2ban service failed to start!"
    exit 1
fi

echo "✓ fail2ban is running with jails:"
sudo fail2ban-client status

echo "=== Log Rotation Configuration ==="

# Configure aggressive log rotation to prevent disk space issues
echo "Configuring log rotation for system logs..."
sudo tee /etc/logrotate.d/branchd-system > /dev/null << 'EOF'
/var/log/syslog
/var/log/kern.log
/var/log/ufw.log
{
    # Rotate when file reaches 50MB
    size 50M
    rotate 2

    # Compress old logs
    compress
    delaycompress

    # Don't error if log is missing
    missingok
    notifempty

    # Create new log file after rotation
    create 0640 syslog adm

    # Run rotation as syslog user
    su syslog adm

    # Use date extension for rotated files
    dateext
    dateformat -%Y%m%d-%s

    postrotate
        # Reload rsyslog to write to new file
        /usr/lib/rsyslog/rsyslog-rotate
    endscript
}
EOF

# Configure log rotation for branchd restore logs
echo "Configuring log rotation for branchd restore logs..."
sudo tee /etc/logrotate.d/branchd-restore > /dev/null << 'EOF'
/var/log/branchd/restore-*.log
{
    # Rotate when file reaches 50MB
    size 50M
    rotate 2

    # Compress old logs
    compress
    delaycompress

    # Don't error if log is missing
    missingok
    notifempty

    # Create new log file after rotation
    create 0644 root root

    # Run rotation as root user
    su root root

    # Use date extension for rotated files
    dateext
    dateformat -%Y%m%d-%s
}
EOF

# Configure journald to limit size
echo "Configuring journald size limits..."
sudo mkdir -p /etc/systemd/journald.conf.d
sudo tee /etc/systemd/journald.conf.d/size-limit.conf > /dev/null << 'EOF'
[Journal]
SystemMaxUse=200M
SystemKeepFree=1G
SystemMaxFileSize=50M
RuntimeMaxUse=100M
RuntimeKeepFree=100M
RuntimeMaxFileSize=50M
MaxRetentionSec=7day
EOF

# Restart journald to apply new settings
sudo systemctl restart systemd-journald

# Force an immediate rotation and cleanup
echo "Running initial log rotation..."
sudo logrotate -f /etc/logrotate.d/branchd-system
sudo logrotate -f /etc/logrotate.d/branchd-restore || true

# Verify configurations
echo "Verifying log rotation configuration..."
if ! sudo logrotate -d /etc/logrotate.d/branchd-system > /dev/null 2>&1; then
    echo "ERROR: branchd-system logrotate configuration is invalid!"
    exit 1
fi

echo "✓ Log rotation configured (50MB limit per log, compressed)"

echo "=== UFW Firewall Configuration ==="

# Configure UFW base rules
echo "Setting up UFW firewall rules..."
sudo ufw --force reset
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp comment 'SSH access'
sudo ufw allow 80/tcp comment 'Caddy HTTP to HTTPS redirect'
sudo ufw allow 443/tcp comment 'Caddy HTTPS (API + UI)'
sudo ufw logging full
sudo ufw --force enable

# Ensure UFW service starts on boot
sudo systemctl enable ufw

echo "UFW firewall status:"
sudo ufw status verbose

# Validate UFW configuration
if ! sudo ufw status | grep -q "Status: active"; then
    echo "ERROR: UFW is not active!"
    exit 1
fi

if ! systemctl is-enabled ufw >/dev/null 2>&1; then
    echo "ERROR: UFW service is not enabled for boot!"
    exit 1
fi

echo "✓ UFW configuration validated"

echo "=== Automatic Security Updates Configuration ==="

# Configure unattended-upgrades with Sunday-only reboots
echo "Configuring unattended-upgrades..."
sudo tee /etc/apt/apt.conf.d/50unattended-upgrades > /dev/null << 'EOF'
// Allowed update origins - security updates only
Unattended-Upgrade::Allowed-Origins {
    "${distro_id}:${distro_codename}-security";
    "${distro_id}ESMApps:${distro_codename}-apps-security";
    "${distro_id}ESM:${distro_codename}-infra-security";
};

// Don't upgrade to development releases
Unattended-Upgrade::DevRelease "false";

// Fix interrupted dpkg processes
Unattended-Upgrade::AutoFixInterruptedDpkg "true";

// Use minimal steps for safer upgrades
Unattended-Upgrade::MinimalSteps "true";

// Remove unused dependencies
Unattended-Upgrade::Remove-Unused-Dependencies "true";

// Enable automatic reboot when required (controlled by our script)
Unattended-Upgrade::Automatic-Reboot "true";

// Reboot time (3 AM)
Unattended-Upgrade::Automatic-Reboot-Time "03:00";

// Only reboot if no users are logged in
Unattended-Upgrade::Automatic-Reboot-WithUsers "false";

// Disable email notifications (customer VMs are temporary/disposable)
Unattended-Upgrade::Mail "";
Unattended-Upgrade::MailOnlyOnError "false";
EOF

# Create script to control reboot timing - only allow reboots on Sundays
sudo tee /usr/local/bin/reboot-if-sunday.sh > /dev/null << 'EOF'
#!/bin/bash
# Only allow reboots on Sundays (day 7 of the week)
if [ "$(date +%u)" = "7" ]; then
    # It's Sunday, allow reboot
    echo "Sunday detected, proceeding with reboot..."
    /sbin/shutdown -r now
else
    # Not Sunday, skip reboot but log the attempt
    echo "Reboot required but not Sunday (today is day $(date +%u)), deferring until Sunday..."
    exit 1
fi
EOF

sudo chmod +x /usr/local/bin/reboot-if-sunday.sh

# Configure unattended-upgrades to use custom reboot script
echo 'Unattended-Upgrade::Automatic-Reboot-Command "/usr/local/bin/reboot-if-sunday.sh";' | sudo tee -a /etc/apt/apt.conf.d/50unattended-upgrades > /dev/null

# Enable automatic updates
echo "Enabling automatic updates..."
sudo tee /etc/apt/apt.conf.d/20auto-upgrades > /dev/null << 'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Download-Upgradeable-Packages "1";
APT::Periodic::AutocleanInterval "7";
APT::Periodic::Unattended-Upgrade "1";
EOF

echo "✓ Automatic security updates configured (Sunday-only reboots)"

echo "=== Redis Installation ==="

# Install Redis for Asynq task queue
echo "Installing Redis..."
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" redis-server

# Configure Redis for Asynq task queue
echo "Configuring Redis for Asynq task queue..."

# Create Redis data directory
sudo mkdir -p /var/lib/redis
sudo chown redis:redis /var/lib/redis
sudo chmod 770 /var/lib/redis

# Create Redis log directory
sudo mkdir -p /var/log/redis
sudo chown redis:redis /var/log/redis

sudo tee /etc/redis/redis.conf > /dev/null <<'EOF'
# Network
bind 127.0.0.1 ::1
protected-mode yes
port 6379

# General
daemonize no
supervised systemd
pidfile /var/run/redis/redis-server.pid
loglevel notice
logfile /var/log/redis/redis-server.log

# Snapshotting - disable RDB snapshots (using AOF instead)
save ""

# Replication
replica-serve-stale-data yes
replica-read-only yes

# Persistence - AOF for durability
appendonly yes
appendfilename "appendonly.aof"
appendfsync everysec
no-appendfsync-on-rewrite no
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb

# Memory management
maxmemory 256mb
maxmemory-policy allkeys-lru

# Working directory
dir /var/lib/redis

# Security
requirepass ""

# Clients
maxclients 10000
EOF

# Enable and start Redis
echo "Enabling and starting Redis service..."
sudo systemctl enable redis-server
sudo systemctl restart redis-server

# Wait for Redis to fully start
sleep 3

# Verify Redis is running
echo "Verifying Redis installation..."
if ! redis-cli ping | grep -q "PONG"; then
    echo "ERROR: Redis is not responding!"
    exit 1
fi
echo "✓ Redis installed and running on localhost:6379"

echo "=== Caddy Installation ==="

# Install dependencies for Caddy
echo "Installing Caddy dependencies..."
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" debian-keyring debian-archive-keyring apt-transport-https curl

# Add Caddy GPG key and repository
echo "Adding Caddy repository..."
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | \
    sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | \
    sudo tee /etc/apt/sources.list.d/caddy-stable.list

# Install Caddy
sudo apt-get update
sudo apt-get install -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" caddy

# Create Caddyfile for serving web UI + reverse proxy to API
echo "Configuring Caddy..."
sudo tee /etc/caddy/Caddyfile > /dev/null <<'EOF'
# Branchd web UI and API reverse proxy
{
    # Global options
    # Admin API on localhost only (required for zero-downtime reloads)
    admin localhost:2019
}

# HTTP to HTTPS redirect
:80 {
    redir https://{host}{uri} permanent
}

# HTTPS with self-signed certificate
:443 {
    # Use explicit self-signed certificate
    tls /etc/postgresql-common/ssl/server.crt /etc/postgresql-common/ssl/server.key

    # Logging (to stdout, captured by systemd journal)
    log {
        format json
    }

    # API endpoints
    handle /api/* {
        reverse_proxy localhost:8080
    }

    # Health check endpoint
    handle /health {
        reverse_proxy localhost:8080
    }

    # Static web UI files
    handle /* {
        root * /var/www/branchd
        try_files {path} /index.html
        file_server

        # Security headers
        header {
            X-Content-Type-Options "nosniff"
            X-Frame-Options "DENY"
            X-XSS-Protection "1; mode=block"
            Referrer-Policy "strict-origin-when-cross-origin"
            Strict-Transport-Security "max-age=31536000; includeSubDomains"
        }
    }

    # Error handling
    handle_errors {
        respond "{http.error.status_code} {http.error.status_text}"
    }
}
EOF

# Make certificates readable by caddy user
echo "Setting certificate permissions for Caddy..."
sudo chmod 644 /etc/postgresql-common/ssl/server.crt
sudo chmod 644 /etc/postgresql-common/ssl/server.key

# Create web root directory (will be populated by branchd installation)
sudo mkdir -p /var/www/branchd
sudo chown caddy:caddy /var/www/branchd

# Validate Caddyfile syntax
echo "Validating Caddyfile..."
if ! sudo caddy validate --config /etc/caddy/Caddyfile; then
    echo "ERROR: Invalid Caddyfile configuration!"
    exit 1
fi

# Enable Caddy service
echo "Enabling Caddy service..."
sudo systemctl enable caddy

echo "✓ Caddy installed and configured (will start on first boot)"

echo "=== Branchd Installation ==="

# Hardcoded for ARM64 Ubuntu 24.04 deployment
BRANCHD_ARCH="arm64"

# Download latest release from GitHub
GITHUB_REPO="branchd-dev/branchd"
RELEASE_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"

echo "Fetching latest release information..."
RELEASE_JSON=$(curl -sL "$RELEASE_URL")
RELEASE_TAG=$(echo "$RELEASE_JSON" | jq -r '.tag_name')
BUNDLE_NAME="branchd-linux-${BRANCHD_ARCH}.tar.gz"
DOWNLOAD_URL=$(echo "$RELEASE_JSON" | jq -r ".assets[] | select(.name == \"${BUNDLE_NAME}\") | .browser_download_url")
CHECKSUM_URL=$(echo "$RELEASE_JSON" | jq -r ".assets[] | select(.name == \"${BUNDLE_NAME}.sha256\") | .browser_download_url")

if [ -z "$RELEASE_TAG" ] || [ -z "$DOWNLOAD_URL" ]; then
    echo "ERROR: Failed to fetch release information from GitHub"
    echo "Please check that releases exist at https://github.com/${GITHUB_REPO}/releases"
    exit 1
fi

echo "Latest release: $RELEASE_TAG"
echo "Downloading Branchd bundle..."

# Download bundle and checksum
cd /tmp
curl -fsSL -o "$BUNDLE_NAME" "$DOWNLOAD_URL"
curl -fsSL -o "${BUNDLE_NAME}.sha256" "$CHECKSUM_URL"

# Verify checksum
echo "Verifying checksum..."
if ! sha256sum -c "${BUNDLE_NAME}.sha256"; then
    echo "ERROR: Checksum verification failed!"
    rm -f "$BUNDLE_NAME" "${BUNDLE_NAME}.sha256"
    exit 1
fi
echo "✓ Checksum verified"

# Extract bundle
echo "Extracting Branchd bundle..."
tar -xzf "$BUNDLE_NAME"

# Verify extracted files
BUNDLE_DIR="branchd-${BRANCHD_ARCH}"
if [ ! -f "${BUNDLE_DIR}/server" ] || [ ! -f "${BUNDLE_DIR}/worker" ]; then
    echo "ERROR: Server or worker binary not found in bundle!"
    exit 1
fi

# Install Go binaries
echo "Installing Branchd binaries..."
sudo install -m 755 "${BUNDLE_DIR}/server" /usr/local/bin/branchd-server
sudo install -m 755 "${BUNDLE_DIR}/worker" /usr/local/bin/branchd-worker

# Verify binaries
echo "Verifying installed binaries..."
if [ -x /usr/local/bin/branchd-server ]; then
    echo "✓ Server binary installed"
else
    echo "ERROR: Server binary not found or not executable!"
    exit 1
fi
if [ -x /usr/local/bin/branchd-worker ]; then
    echo "✓ Worker binary installed"
else
    echo "ERROR: Worker binary not found or not executable!"
    exit 1
fi

# Install web UI
echo "Installing web UI..."
if [ -d "${BUNDLE_DIR}/web" ]; then
    sudo rm -rf /var/www/branchd/*
    sudo cp -r "${BUNDLE_DIR}/web"/* /var/www/branchd/
    sudo chown -R caddy:caddy /var/www/branchd
    echo "✓ Web UI installed to /var/www/branchd"
else
    echo "WARNING: Web UI files not found in bundle"
fi

# Cleanup
echo "Cleaning up installation files..."
cd /
rm -rf /tmp/branchd-* /tmp/"$BUNDLE_NAME" /tmp/"${BUNDLE_NAME}.sha256"

echo "✓ Branchd $RELEASE_TAG installed successfully"

# Create data directory
echo "Creating Branchd data directory..."
sudo mkdir -p /data
sudo chmod 755 /data

# Create systemd service for branchd-server
echo "Creating branchd-server systemd service..."
sudo tee /etc/systemd/system/branchd-server.service > /dev/null <<'EOF'
[Unit]
Description=Branchd API Server
Documentation=https://branchd.dev
After=network-online.target redis-server.service
Wants=network-online.target redis-server.service
Requires=redis-server.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/usr/local/bin

# Binary
ExecStart=/usr/local/bin/branchd-server

# Restart policy
Restart=always
RestartSec=5s
StartLimitInterval=0

# Environment
Environment="SERVER_ADDRESS=127.0.0.1:8080"
Environment="DATABASE_URL=/data/branchd.sqlite"
Environment="REDIS_ADDRESS=localhost:6379"
Environment="LOG_LEVEL=info"
Environment="LOG_FORMAT=json"
Environment="GIN_MODE=release"

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=branchd-server

# Resource limits
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# Create systemd service for branchd-worker
echo "Creating branchd-worker systemd service..."
sudo tee /etc/systemd/system/branchd-worker.service > /dev/null <<'EOF'
[Unit]
Description=Branchd Background Worker
Documentation=https://branchd.dev
After=network-online.target redis-server.service branchd-server.service
Wants=network-online.target redis-server.service
Requires=redis-server.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/usr/local/bin

# Binary
ExecStart=/usr/local/bin/branchd-worker

# Restart policy
Restart=always
RestartSec=5s
StartLimitInterval=0

# Environment
Environment="DATABASE_URL=/data/branchd.sqlite"
Environment="REDIS_ADDRESS=localhost:6379"
Environment="LOG_LEVEL=info"
Environment="LOG_FORMAT=json"

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=branchd-worker

# Resource limits
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# Reload systemd
echo "Reloading systemd daemon..."
sudo systemctl daemon-reload

# Enable services
echo "Enabling Branchd services..."
sudo systemctl enable branchd-server.service
sudo systemctl enable branchd-worker.service

# Start Caddy
echo "Starting Caddy..."
sudo systemctl start caddy

# Wait for Caddy to fully start, then reload to ensure config is loaded
sleep 2
echo "Reloading Caddy configuration..."
sudo systemctl reload caddy

# Verify Caddy is running and serving on both ports
echo "Verifying Caddy is serving on ports 80 and 443..."
if ! sudo ss -tlnp | grep -q ':80.*caddy' || ! sudo ss -tlnp | grep -q ':443.*caddy'; then
    echo "ERROR: Caddy is not listening on ports 80 and/or 443!"
    echo "Current listening ports:"
    sudo ss -tlnp | grep caddy || echo "No Caddy ports found"
    exit 1
fi
echo "✓ Caddy is serving on HTTP (80) and HTTPS (443)"

# Verify services are enabled
if ! systemctl is-enabled branchd-server.service | grep -q "enabled"; then
    echo "ERROR: branchd-server service is not enabled!"
    exit 1
fi
if ! systemctl is-enabled branchd-worker.service | grep -q "enabled"; then
    echo "ERROR: branchd-worker service is not enabled!"
    exit 1
fi

echo "✓ Branchd systemd services created and enabled"

echo "=== Creating ZFS Pool and PostgreSQL Cluster ==="

# Execute ZFS pool creation script directly
echo "Running ZFS pool creation script..."
if ! sudo /usr/local/bin/create-tank-pool.sh; then
    echo "ERROR: Failed to create ZFS pool!"
    echo "Check available block devices with: lsblk"
    echo "You can run the script manually later: sudo /usr/local/bin/create-tank-pool.sh"
    exit 1
fi

echo "✓ ZFS pool and PostgreSQL cluster created successfully"

echo "=== Starting Branchd Services ==="

# Start Branchd services
echo "Starting branchd-server..."
sudo systemctl start branchd-server

echo "Starting branchd-worker..."
sudo systemctl start branchd-worker

# Wait a moment for services to start
sleep 3

# Verify services are running
if ! systemctl is-active --quiet branchd-server; then
    echo "WARNING: branchd-server failed to start!"
    echo "Check logs with: journalctl -u branchd-server -n 50"
fi

if ! systemctl is-active --quiet branchd-worker; then
    echo "WARNING: branchd-worker failed to start!"
    echo "Check logs with: journalctl -u branchd-worker -n 50"
fi

echo "✓ All services started"

echo ""
echo "=========================================="
echo "=== Branchd Setup Complete! ==="
echo "=========================================="
echo ""
echo "Summary:"
echo "  ✓ PostgreSQL ${PG_VERSION} running (port 54${PG_VERSION})"
echo "  ✓ ZFS pool 'tank' created and mounted"
echo "  ✓ Security: fail2ban, UFW firewall active"
echo "  ✓ Automatic updates: enabled (Sunday-only reboots at 3 AM)"
echo "  ✓ Redis: running on localhost:6379"
echo "  ✓ Systemd services: created and enabled"
echo "  ✓ Caddy: running (HTTP:80 → HTTPS:443)"
echo "  ✓ Branchd: $RELEASE_TAG running"
echo ""
echo "🎉 Setup complete! Access your Branchd instance at:"
echo ""
echo "    https://$(hostname -I | awk '{print $1}')/"
echo ""
echo "    (Accept self-signed certificate warning)"
echo ""
echo "Next steps:"
echo "  1. Open the URL above in your browser"
echo "  2. Complete first-time setup by creating an admin account"
echo "  3. Configure your source PostgreSQL database"
echo "  4. Start creating database branches!"

echo ""
echo "Useful commands:"
echo "  - View server logs: journalctl -u branchd-server -f"
echo "  - View worker logs: journalctl -u branchd-worker -f"
echo "  - Check service status: systemctl status branchd-server branchd-worker"
echo "  - Check ZFS pool: zpool status tank"
echo "  - Check PostgreSQL: systemctl status postgresql@${PG_VERSION}-main"
echo ""
