#!/bin/bash
# Immediate output BEFORE set -eu to ensure we always get something
echo "BRANCH_CREATION_SCRIPT_LOADED=true" >&2

set -eu  # Exit on error and undefined variables, but no pipefail

# Branchd Branch Creation Script
#
# Creates a new PostgreSQL branch from a running primary database.
#
# Flow:
# 1. Verify the source database is ready (accepting connections)
# 2. Find available port for the branch
# 3. Create ZFS snapshot of the source data directory
# 4. Clone snapshot to new mountpoint for the branch
# 5. Clean up source-specific config and recovery files
# 6. Start PostgreSQL service (as independent primary)
# 7. Wait for PostgreSQL to be ready and create database user
# 8. Apply custom PostgreSQL configuration if provided
#
# Note: The source is a primary database created via pg_dump/restore.
# The ZFS clone starts as an independent primary (no promotion or WAL replay needed).

# Output after set -eu
echo "BRANCH_CREATION_STARTED=true"

# Input parameters
BRANCH_NAME="{{.BranchName}}"
DATASET_NAME="{{.DatasetName}}"
USER="{{.User}}"
PASSWORD="{{.Password}}"
PG_VERSION="{{.PgVersion}}"
CUSTOM_POSTGRESQL_CONF="{{.CustomPostgresqlConf}}"

echo "DEBUG: Parameters loaded successfully"

# Derive PostgreSQL port from version (hardcoded ports: PG14→5414, PG15→5415, etc.)
RESTORE_PORT="54${PG_VERSION}"
echo "DEBUG: PostgreSQL version ${PG_VERSION}, using port ${RESTORE_PORT}"

# Configuration
PORT_RANGE_START=15432
PORT_RANGE_END=16432

BRANCH_MOUNTPOINT="/opt/branchd/${BRANCH_NAME}"
# Branch PostgreSQL data directory (in 'main' subdirectory after ZFS clone)
BRANCH_PGDATA="${BRANCH_MOUNTPOINT}/main"
PORT_ALLOCATION_LOCK="/tmp/branchd-port-allocation.lock"
SERVICE_NAME="branchd-branch-${BRANCH_NAME}"

echo "Creating branch: ${BRANCH_NAME} from dataset: ${DATASET_NAME}"

# Cleanup function to be called on exit
# cleanup() {
#     local exit_code=$?
#     local signal_received=${1:-}

#     # Clean up if script failed OR was interrupted by signal
#     if [ $exit_code -ne 0 ] || [ -n "$signal_received" ]; then
#         if [ -n "$signal_received" ]; then
#             echo "Script interrupted by signal $signal_received, cleaning up..."
#         else
#             echo "Script failed with exit code $exit_code, cleaning up..."
#         fi

#         # Stop and remove systemd service first
#         # Check if service file exists (avoid pipefail issues with grep)
#         if systemctl list-unit-files "${SERVICE_NAME}.service" 2>/dev/null | grep -q .; then
#             echo "Stopping and removing systemd service..."
#             sudo systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
#             sudo systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
#             sudo rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
#             sudo systemctl daemon-reload
#         fi

#         # Kill any remaining PostgreSQL processes for this branch
#         if [ -n "${BRANCH_NAME:-}" ]; then
#             echo "Killing any remaining PostgreSQL processes..."
#             sudo pkill -f "postgres.*${BRANCH_NAME}" 2>/dev/null || true
#             sleep 2  # Give processes time to exit
#         fi

#         # Remove ZFS snapshot and any dependent clones recursively
#         if sudo zfs list -t snapshot "${DATASET_NAME}@${BRANCH_NAME}" >/dev/null 2>&1; then
#             echo "Removing ZFS snapshot and dependent clones..."
#             sudo zfs destroy -R "${DATASET_NAME}@${BRANCH_NAME}" || echo "Warning: Failed to remove snapshot and clones"

#             # Remove leftover mountpoint directory (zfs destroy unmounts but leaves directory)
#             if [ -d "${BRANCH_MOUNTPOINT}" ]; then
#                 echo "Removing mountpoint directory ${BRANCH_MOUNTPOINT}..."
#                 sudo rmdir "${BRANCH_MOUNTPOINT}" 2>/dev/null || sudo rm -rf "${BRANCH_MOUNTPOINT}"
#             fi
#         fi

#         # Close UFW port if it was opened
#         if [ -n "${AVAILABLE_PORT:-}" ]; then
#             echo "Closing UFW port ${AVAILABLE_PORT}..."
#             sudo ufw --force delete allow "${AVAILABLE_PORT}/tcp" 2>/dev/null || true
#         fi
#     fi

#     # Always remove lock files if we created them
#     rm -f "${PORT_ALLOCATION_LOCK}" 2>/dev/null || true
# }

# Signal handlers for graceful cleanup
# cleanup_on_signal() {
#     cleanup "$1"
#     exit 1
# }

# Find available port with locking to prevent race conditions
find_available_port() {
    local selected_port=""
    local lock_acquired=false

    # Use file locking to ensure atomic port allocation
    (
        flock -x 200
        lock_acquired=true
        echo "Acquired port allocation lock, finding available port..."
        # Check if port is being forced (pg_dump/restore refresh scenario)
        if [ -n "${FORCE_PORT:-}" ]; then
            echo "Using forced port: ${FORCE_PORT}"

            # Validate forced port is available
            # Use 'ss' instead of 'netstat' (Ubuntu 24.04 default)
            if ss -ln | grep -q ":${FORCE_PORT} "; then
                echo "BRANCHD_ERROR: Forced port ${FORCE_PORT} is already in use"
                rm -f "${PORT_ALLOCATION_LOCK}" 2>/dev/null || true
                return 1
            fi

            if sudo ufw status numbered | grep -q "${FORCE_PORT}/tcp"; then
                echo "BRANCHD_ERROR: Forced port ${FORCE_PORT} is already reserved in UFW"
                rm -f "${PORT_ALLOCATION_LOCK}" 2>/dev/null || true
                return 1
            fi

            # Reserve the forced port
            if ! sudo ufw allow "${FORCE_PORT}/tcp" >/dev/null 2>&1; then
                echo "BRANCHD_ERROR: Failed to reserve forced port ${FORCE_PORT}"
                rm -f "${PORT_ALLOCATION_LOCK}" 2>/dev/null || true
                return 1
            fi

            echo "Reserved forced port ${FORCE_PORT}"
            rm -f "${PORT_ALLOCATION_LOCK}" 2>/dev/null || true
            echo "Released port allocation lock"
            echo "$FORCE_PORT"
            return 0
        fi


        # Create randomized port list to distribute load
        local ports=($(seq ${PORT_RANGE_START} ${PORT_RANGE_END} | shuf))

        for port in "${ports[@]}"; do
            # Check if port is listening (active PostgreSQL)
            # Use 'ss' instead of 'netstat' (Ubuntu 24.04 default)
            if ss -ln | grep -q ":${port} "; then
                continue
            fi

            # Check if port is open in UFW (might be reserved by a stopped branch)
            if sudo ufw status numbered | grep -q "${port}/tcp"; then
                continue
            fi

            # Port appears available - immediately reserve it in UFW to claim it
            # WHY: UFW acts as our port reservation system to prevent race conditions
            # between concurrent branch creations
            if sudo ufw allow "${port}/tcp" >/dev/null 2>&1; then
                selected_port=$port
                echo "Reserved port ${selected_port}"
                break
            fi
        done

        if [ -z "$selected_port" ]; then
            echo "BRANCHD_ERROR: No available ports in range ${PORT_RANGE_START}-${PORT_RANGE_END}"
            # Clean up lock file before returning error
            rm -f "${PORT_ALLOCATION_LOCK}" 2>/dev/null || true
            return 1
        fi

        # Clean up lock file immediately after successful allocation
        rm -f "${PORT_ALLOCATION_LOCK}" 2>/dev/null || true
        echo "Released port allocation lock"

        echo "$selected_port"
    ) 200>"${PORT_ALLOCATION_LOCK}"
}

# Set up cleanup traps
# trap cleanup EXIT
# trap 'cleanup_on_signal SIGTERM' TERM
# trap 'cleanup_on_signal SIGINT' INT

# Check if source database is ready to accept connections
echo "Checking if source database is ready to accept connections on port ${RESTORE_PORT}..."
if ! sudo -u postgres pg_isready -p "${RESTORE_PORT}" >/dev/null 2>&1; then
    echo "BRANCHD_ERROR:DATABASE_NOT_READY: Source database is not accepting connections on port ${RESTORE_PORT}"
    echo "Please wait for the database to start and try again"
    exit 1
fi

echo "Source database is ready to accept connections on port ${RESTORE_PORT}"

# Find available port with locking
echo "Finding available port..."
AVAILABLE_PORT=$(find_available_port | tail -1)
if [ $? -ne 0 ] || [ -z "${AVAILABLE_PORT}" ]; then
    echo "BRANCHD_ERROR: Failed to find available port"
    exit 1
fi

echo "Found available port: ${AVAILABLE_PORT}"

# Create ZFS snapshot
# WHY: Snapshot preserves the current database state for branching
echo "Creating ZFS snapshot..."
if sudo zfs list -t snapshot "${DATASET_NAME}@${BRANCH_NAME}" >/dev/null 2>&1; then
    echo "ZFS snapshot already exists, skipping..."
else
    sudo zfs snapshot "${DATASET_NAME}@${BRANCH_NAME}"
    echo "ZFS snapshot created successfully"
fi

# Create ZFS clone with direct mountpoint
echo "Creating ZFS clone..."
if sudo zfs list "tank/${BRANCH_NAME}" >/dev/null 2>&1; then
    echo "ZFS clone already exists, ensuring it's mounted..."

    # Ensure systemd ignore property is set
    sudo zfs set org.openzfs.systemd:ignore=on "tank/${BRANCH_NAME}"

    # Check if already mounted using ZFS
    if [ "$(sudo zfs get -H -o value mounted tank/${BRANCH_NAME})" = "yes" ]; then
        echo "ZFS clone already mounted"
    else
        echo "ZFS clone exists but not mounted, mounting now..."
        # Unmount first to be safe (ignore errors)
        sudo zfs unmount "tank/${BRANCH_NAME}" 2>/dev/null || true
        # Create mountpoint if it doesn't exist
        if [ ! -d "${BRANCH_MOUNTPOINT}" ]; then
            sudo mkdir -p "${BRANCH_MOUNTPOINT}"
        fi
        # Mount the clone
        if ! sudo zfs mount "tank/${BRANCH_NAME}"; then
            echo "BRANCHD_ERROR: Failed to mount existing ZFS clone"
            exit 1
        fi
        echo "ZFS clone mounted successfully"
    fi
else
    # Create clone - ZFS should automatically mount it
    # Set org.openzfs.systemd:ignore to prevent systemd from managing this mount
    echo "Creating ZFS clone with automatic mount..."
    sudo zfs clone -o mountpoint="${BRANCH_MOUNTPOINT}" -o org.openzfs.systemd:ignore=on "${DATASET_NAME}@${BRANCH_NAME}" "tank/${BRANCH_NAME}"

    # Verify the clone was mounted using ZFS
    if [ "$(sudo zfs get -H -o value mounted tank/${BRANCH_NAME})" != "yes" ]; then
        echo "Clone created but not automatically mounted, mounting explicitly..."
        # Create mountpoint if it doesn't exist
        if [ ! -d "${BRANCH_MOUNTPOINT}" ]; then
            sudo mkdir -p "${BRANCH_MOUNTPOINT}"
        fi
        # Mount the clone
        if ! sudo zfs mount "tank/${BRANCH_NAME}"; then
            echo "BRANCHD_ERROR: Failed to mount ZFS clone after creation"
            exit 1
        fi
    fi
    echo "ZFS clone created and mounted successfully"
fi

# Clean up PostgreSQL files in the clone
echo "Cleaning up PostgreSQL files..."
sudo -u postgres rm -f "${BRANCH_PGDATA}/postmaster.pid"
sudo -u postgres rm -f "${BRANCH_PGDATA}/postgresql.auto.conf"

# Copy config files from system location to data directory
# Ubuntu's pg_createcluster stores configs in /etc/postgresql/, but branches need them in the data dir
echo "Copying PostgreSQL config files to data directory..."
sudo -u postgres cp "/etc/postgresql/${PG_VERSION}/main/postgresql.conf" "${BRANCH_PGDATA}/postgresql.conf"
sudo -u postgres cp "/etc/postgresql/${PG_VERSION}/main/pg_hba.conf" "${BRANCH_PGDATA}/pg_hba.conf.bak"
sudo -u postgres cp "/etc/postgresql/${PG_VERSION}/main/pg_ident.conf" "${BRANCH_PGDATA}/pg_ident.conf"

# Minimal update to postgresql.conf for the new port.
# Note: SSL configuration is inherited from the main cluster via ZFS clone
echo "Updating postgresql.conf..."
sudo -u postgres sed -i "s/^#*port = .*/port = ${AVAILABLE_PORT}/" "${BRANCH_PGDATA}/postgresql.conf"
sudo -u postgres sed -i "s/^#*listen_addresses = .*/listen_addresses = '*'/" "${BRANCH_PGDATA}/postgresql.conf"
sudo -u postgres sed -i "s/^#*checkpoint_timeout = .*/checkpoint_timeout = 15min/" "${BRANCH_PGDATA}/postgresql.conf"
# CRITICAL: Update data_directory to point to branch data dir, not parent
sudo -u postgres sed -i "s|^#*data_directory = .*|data_directory = '${BRANCH_PGDATA}'|" "${BRANCH_PGDATA}/postgresql.conf"
# CRITICAL: Point hba_file to the branch's pg_hba.conf, not the system default
sudo -u postgres sed -i "s|^#*hba_file = .*|hba_file = '${BRANCH_PGDATA}/pg_hba.conf'|" "${BRANCH_PGDATA}/postgresql.conf"
# CRITICAL: Point ident_file to the branch's pg_ident.conf
sudo -u postgres sed -i "s|^#*ident_file = .*|ident_file = '${BRANCH_PGDATA}/pg_ident.conf'|" "${BRANCH_PGDATA}/postgresql.conf"

# Update pg_hba.conf for security
echo "Updating pg_hba.conf for security..."
sudo -u postgres tee "${BRANCH_PGDATA}/pg_hba.conf" > /dev/null << EOF
# TYPE  DATABASE        USER            ADDRESS                 METHOD

# Allow local socket connections
local   all             all                                     peer

# Require SSL for generated user (without client certificates)
hostssl all             ${USER}         0.0.0.0/0               scram-sha-256
hostssl all             ${USER}         ::/0                    scram-sha-256

# Deny all other connections
host    all             all             0.0.0.0/0               reject
host    all             all             ::/0                    reject
EOF

sudo chown postgres:postgres -R "${BRANCH_MOUNTPOINT}"

# Create systemd service for the branch
echo "Creating systemd service for branch ${BRANCH_NAME}..."
PG_CTL_PATH="/usr/lib/postgresql/${PG_VERSION}/bin/pg_ctl"

sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" > /dev/null << EOF
[Unit]
Description=Branchd Branch (${BRANCH_NAME})
After=network.target

[Service]
Type=forking
User=postgres
# Ensure ZFS mount is present before starting PostgreSQL (run as root with +)
ExecStartPre=+/usr/bin/sh -c '/usr/sbin/zfs mount tank/${BRANCH_NAME} 2>/dev/null || true'
ExecStart=${PG_CTL_PATH} start -D ${BRANCH_PGDATA} -l ${BRANCH_PGDATA}/postgresql.log
ExecStop=${PG_CTL_PATH} stop -D ${BRANCH_PGDATA} -m immediate
ExecReload=/bin/kill -HUP \$MAINPID
KillMode=mixed
KillSignal=SIGINT
TimeoutStartSec=30
TimeoutStopSec=30
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

# Verify ZFS mount is stable and accessible by postgres user
# This ensures the mount is fully propagated before we try to use it
echo "Verifying ZFS mount stability..."
for i in {1..20}; do
    if sudo -u postgres test -r "${BRANCH_PGDATA}/PG_VERSION" 2>/dev/null; then
        echo "ZFS mount verified accessible to postgres user after ${i} attempts"
        break
    fi

    if [ $i -eq 20 ]; then
        echo "ERROR: ZFS mount not accessible to postgres user after 20 attempts"
        echo "Mount status:"
        mount | grep "${BRANCH_MOUNTPOINT}" || echo "Mount not found in mount table"
        echo "Directory status:"
        ls -la "${BRANCH_MOUNTPOINT}" || echo "Cannot access mountpoint"
        exit 1
    fi

    sleep 0.2
done

# Force systemd to reload and recognize the new mount and service
echo "Reloading systemd daemon..."
sudo systemctl daemon-reload

# Verify ZFS mount is still present after daemon-reload
# (daemon-reload can sometimes trigger mount/umount events)
echo "Verifying ZFS mount after daemon-reload..."
if [ "$(sudo zfs get -H -o value mounted tank/${BRANCH_NAME})" != "yes" ]; then
    echo "WARNING: ZFS mount was unmounted during daemon-reload, remounting..."
    sudo zfs mount "tank/${BRANCH_NAME}"

    # Verify mount succeeded
    if [ "$(sudo zfs get -H -o value mounted tank/${BRANCH_NAME})" != "yes" ]; then
        echo "BRANCHD_ERROR: Failed to remount ZFS dataset after daemon-reload"
        exit 1
    fi
    echo "ZFS mount restored successfully"
else
    echo "ZFS mount still present after daemon-reload"
fi

# Additional short delay to ensure mount is visible to systemd's new service context
sleep 1

# Start the service
echo "Starting PostgreSQL service ${SERVICE_NAME}..."

# Enable service if not already enabled
if ! sudo systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
    sudo systemctl enable "${SERVICE_NAME}"
    echo "Service enabled"
else
    echo "Service already enabled, skipping"
fi

# Start service if not already running
if ! sudo systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    # Final mount verification right before starting service
    # (systemctl operations can trigger unmount)
    echo "Final mount verification before starting service..."
    if [ "$(sudo zfs get -H -o value mounted tank/${BRANCH_NAME})" != "yes" ]; then
        echo "WARNING: ZFS mount was unmounted before service start, remounting..."
        sudo zfs mount "tank/${BRANCH_NAME}"

        if [ "$(sudo zfs get -H -o value mounted tank/${BRANCH_NAME})" != "yes" ]; then
            echo "BRANCHD_ERROR: Failed to remount ZFS dataset before service start"
            exit 1
        fi
        echo "ZFS mount restored before service start"
    else
        echo "ZFS mount confirmed present before service start"
    fi

    # Force remount to ensure it stays mounted during systemctl start
    # (systemctl can unmount ZFS datasets even with org.openzfs.systemd:ignore=on)
    echo "Force remounting to ensure stability..."
    sudo zfs unmount "tank/${BRANCH_NAME}" 2>/dev/null || true
    sudo zfs mount "tank/${BRANCH_NAME}"

    if [ "$(sudo zfs get -H -o value mounted tank/${BRANCH_NAME})" != "yes" ]; then
        echo "BRANCHD_ERROR: Mount verification failed after force remount"
        exit 1
    fi
    echo "Mount confirmed stable"

    # Start service immediately after mount
    sudo systemctl start "${SERVICE_NAME}"
    echo "Service started"
else
    echo "Service already running, skipping start"
fi

# Wait for PostgreSQL to be ready and create user
echo "Waiting for PostgreSQL to be ready on port ${AVAILABLE_PORT}..."

# Function to check PostgreSQL readiness
wait_for_postgres() {
    local port="$1"
    local max_attempts="${2:-60}"  # Default 60 seconds
    local attempt=1

    while [ $attempt -le $max_attempts ]; do
        set +e  # Temporarily disable exit on error for pg_isready
        sudo -u postgres pg_isready -p "${port}" >/dev/null 2>&1
        local pg_ready_exit_code=$?
        set -e  # Re-enable exit on error

        # Exit code 0 means PostgreSQL is ready and accepting connections
        if [ $pg_ready_exit_code -eq 0 ]; then
            echo "PostgreSQL is ready and accepting connections"
            return 0
        fi

        if [ $attempt -eq $max_attempts ]; then
            echo "BRANCHD_ERROR: PostgreSQL not ready on port ${port} within ${max_attempts} seconds"
            return 1
        fi

        echo "PostgreSQL not ready yet (exit code: $pg_ready_exit_code), attempt $attempt/$max_attempts..."
        sleep 1
        attempt=$((attempt + 1))
    done
}

# Wait for PostgreSQL to be ready
if ! wait_for_postgres "${AVAILABLE_PORT}"; then
    exit 1
fi

# Create database user with full privileges
echo "Creating database user '${USER}'..."
if ! sudo -u postgres psql -p "${AVAILABLE_PORT}" -c "
    CREATE USER \"${USER}\" WITH PASSWORD '${PASSWORD}' SUPERUSER;
"; then
    echo "BRANCHD_ERROR: Failed to create user '${USER}' (see error above)"
    exit 1
fi

# Apply custom PostgreSQL configuration if provided
if [ -n "${CUSTOM_POSTGRESQL_CONF}" ]; then
    echo "Applying custom PostgreSQL configuration..."
    echo "DEBUG: CustomPostgresqlConf = '${CUSTOM_POSTGRESQL_CONF}'"

    # Decode and apply each custom setting
    echo "${CUSTOM_POSTGRESQL_CONF}" | base64 -d | while IFS='=' read -r key value; do
        echo "DEBUG: Processing line with key='${key}' value='${value}'"

        # Skip empty lines
        if [ -z "${key}" ] || [ -z "${value}" ]; then
            echo "DEBUG: Skipping empty line"
            continue
        fi

        # Trim whitespace
        key=$(echo "${key}" | xargs)
        value=$(echo "${value}" | xargs)
        echo "DEBUG: After trimming: key='${key}' value='${value}'"

        # Update the setting in postgresql.conf (override default if exists)
        if sudo -u postgres grep -q "^#*${key}\\s*=" "${BRANCH_PGDATA}/postgresql.conf"; then
            echo "DEBUG: Found existing setting for ${key}, updating..."
            sudo -u postgres sed -i "s/^#*${key}\\s*=.*/${key} = ${value}/" "${BRANCH_PGDATA}/postgresql.conf"
            if [ $? -eq 0 ]; then
                echo "Updated ${key} = ${value}"
            else
                echo "DEBUG: sed command failed for ${key}"
            fi
        else
            echo "DEBUG: Setting ${key} not found, adding new..."
            # Add new setting if it doesn't exist
            echo "${key} = ${value}" | sudo -u postgres tee -a "${BRANCH_PGDATA}/postgresql.conf" > /dev/null
            echo "Added ${key} = ${value}"
        fi
    done

    echo "Custom PostgreSQL configuration applied"
    echo "Restarting PostgreSQL to apply configuration changes..."

    # Restart the PostgreSQL service
    if sudo systemctl restart "${SERVICE_NAME}"; then
        echo "PostgreSQL service restarted successfully"

        # Wait for PostgreSQL to be ready again
        if ! wait_for_postgres "${AVAILABLE_PORT}" 30; then
            echo "BRANCHD_ERROR: PostgreSQL not ready after restart"
            exit 1
        fi
        echo "PostgreSQL is ready after restart"
    else
        echo "BRANCHD_ERROR: Failed to restart PostgreSQL service"
        exit 1
    fi
else
    echo "DEBUG: No custom PostgreSQL configuration provided"
fi

echo "USER_CREATION_SUCCESS=true"

# Output port for Go code to parse
echo "BRANCH_PORT=${AVAILABLE_PORT}"
