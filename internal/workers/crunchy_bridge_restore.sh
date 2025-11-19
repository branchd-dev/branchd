#!/bin/bash
# pgBackRest restore script for Crunchy Bridge backups
set -euo pipefail

# Configuration from template
readonly PG_VERSION="{{.PgVersion}}"
readonly PG_PORT="{{.PgPort}}"
readonly DATABASE_NAME="{{.DatabaseName}}"
readonly DATA_DIR="{{.DataDir}}"       # e.g., /opt/branchd/restore_20250915120000/data
readonly PGBACKREST_CONF="{{.PgBackRestConfPath}}"
readonly STANZA_NAME="{{.StanzaName}}"

# Paths
readonly RESTORE_LOG_DIR="/var/log/branchd"
readonly RESTORE_LOG="${RESTORE_LOG_DIR}/restore-${DATABASE_NAME}.log"
readonly RESTORE_PID="${RESTORE_LOG_DIR}/restore-${DATABASE_NAME}.pid"
readonly PG_BIN="/usr/lib/postgresql/${PG_VERSION}/bin"
readonly RESTORE_DATASET_PATH=$(dirname "${DATA_DIR}")  # /opt/branchd/restore_YYYYMMDDHHMMSS
readonly ZFS_DATASET="tank/${DATABASE_NAME}"
readonly SERVICE_NAME="branchd-restore-${DATABASE_NAME}"

# Helper functions
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1"
}

die() {
    log "ERROR: $1" >&2

    # Stop PostgreSQL service if it was started
    # if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    #     log "Stopping PostgreSQL service..."
    #     sudo systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
    # fi

    # Remove systemd service
    # if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
    #     log "Removing systemd service..."
    #     sudo systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
    #     sudo rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    #     sudo systemctl daemon-reload
    # fi

    # Destroy ZFS dataset if it was created
    # if sudo zfs list "${ZFS_DATASET}" >/dev/null 2>&1; then
    #     log "Destroying ZFS dataset..."
    #     sudo zfs destroy -r "${ZFS_DATASET}" 2>/dev/null || log "Warning: Could not destroy ZFS dataset"
    # fi
    log "WARNING: ZFS dataset ${ZFS_DATASET} left intact for debugging (not destroyed)"

    # Clean up pgBackRest config
    # if [ -f "${PGBACKREST_CONF}" ]; then
    #     log "Cleaning up pgBackRest config..."
    #     rm -f "${PGBACKREST_CONF}" 2>/dev/null || log "Warning: Could not remove pgBackRest config"
    # fi
    log "WARNING: pgBackRest config ${PGBACKREST_CONF} left intact for debugging (not deleted)"

    # Write failure marker
    echo '__BRANCHD_RESTORE_FAILED__' >> "${RESTORE_LOG}"
    sync
    sleep 0.5

    # Remove PID file
    rm -f "${RESTORE_PID}" 2>/dev/null || true
    exit 1
}

log "Starting Crunchy Bridge restore: ${DATABASE_NAME}"
log "PostgreSQL version: ${PG_VERSION}, Port: ${PG_PORT}"
log "Data directory: ${DATA_DIR}"
log "Stanza: ${STANZA_NAME}"

# 1. Create ZFS dataset for this restore
log "Creating ZFS dataset: ${ZFS_DATASET}"
if sudo zfs list "${ZFS_DATASET}" >/dev/null 2>&1; then
    log "ZFS dataset already exists, destroying and recreating..."
    sudo zfs destroy -r "${ZFS_DATASET}" || die "Failed to destroy existing ZFS dataset"
fi

sudo zfs create "${ZFS_DATASET}" || die "Failed to create ZFS dataset"
sudo zfs set mountpoint="${RESTORE_DATASET_PATH}" "${ZFS_DATASET}"
log "ZFS dataset created and mounted at ${RESTORE_DATASET_PATH}"

# 2. Create data directory and set ownership
log "Creating data directory..."
sudo mkdir -p "${DATA_DIR}" || die "Failed to create data directory"
sudo chown -R postgres:postgres "${RESTORE_DATASET_PATH}"
log "Data directory created with postgres ownership"

# 3. Restore from Crunchy Bridge using pgBackRest
log "Starting pgBackRest restore from Crunchy Bridge..."
log "Using pgBackRest config: ${PGBACKREST_CONF}"

# Run pgBackRest restore
set +e
sudo -u postgres pgbackrest --config="${PGBACKREST_CONF}" \
    --stanza="${STANZA_NAME}" \
    --pg1-path="${DATA_DIR}" \
    --type=immediate \
    --target-action=promote \
    restore 2>&1
RESTORE_EXIT=$?
set -e

log "pgBackRest restore completed with exit code: ${RESTORE_EXIT}"

if [ ${RESTORE_EXIT} -ne 0 ]; then
    die "pgBackRest restore failed with exit code ${RESTORE_EXIT}"
fi

# 4. Verify PostgreSQL data directory
log "Verifying PostgreSQL data directory..."
if [ ! -f "${DATA_DIR}/PG_VERSION" ]; then
    die "PostgreSQL data directory is invalid (missing PG_VERSION file)"
fi

PG_DATA_VERSION=$(cat "${DATA_DIR}/PG_VERSION")
log "Restored PostgreSQL version: ${PG_DATA_VERSION}"

if [ "${PG_DATA_VERSION}" != "${PG_VERSION}" ]; then
    log "WARNING: Restored version (${PG_DATA_VERSION}) differs from expected version (${PG_VERSION})"
fi

# 5. Configure PostgreSQL for local access
log "Configuring PostgreSQL..."

# Copy TLS certificates (shared across all clusters)
sudo -u postgres cp /etc/postgresql-common/ssl/server.crt "${DATA_DIR}/"
sudo -u postgres cp /etc/postgresql-common/ssl/server.key "${DATA_DIR}/"
# Fix permissions on server.key (PostgreSQL requires 0600)
sudo -u postgres chmod 0600 "${DATA_DIR}/server.key"
sudo -u postgres chmod 0644 "${DATA_DIR}/server.crt"

# Backup original Crunchy Bridge config for reference
sudo -u postgres cp "${DATA_DIR}/postgresql.conf" "${DATA_DIR}/postgresql.conf.crunchybridge"

log "Extracting recovery-critical parameters from postgresql.conf and conf.d..."

# Extract recovery-critical parameters from BOTH postgresql.conf (backup values) and conf.d
# These MUST be >= the primary server's values for recovery to succeed
# We take the MAX of both sources to ensure we meet recovery requirements

# Helper function to get max of two numbers
max_value() {
    local val1=$1
    local val2=$2
    if [ "$val1" -ge "$val2" ]; then
        echo "$val1"
    else
        echo "$val2"
    fi
}

# Extract from main postgresql.conf (has correct backup values)
PG_MAX_CONNECTIONS=$(sudo -u postgres grep -h "^max_connections" "${DATA_DIR}/postgresql.conf" 2>/dev/null | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "100")
PG_MAX_WORKER_PROCESSES=$(sudo -u postgres grep -h "^max_worker_processes" "${DATA_DIR}/postgresql.conf" 2>/dev/null | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "8")
PG_MAX_WAL_SENDERS=$(sudo -u postgres grep -h "^max_wal_senders" "${DATA_DIR}/postgresql.conf" 2>/dev/null | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "10")
PG_MAX_PREPARED_XACTS=$(sudo -u postgres grep -h "^max_prepared_transactions" "${DATA_DIR}/postgresql.conf" 2>/dev/null | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "0")
PG_MAX_LOCKS_PER_XACT=$(sudo -u postgres grep -h "^max_locks_per_transaction" "${DATA_DIR}/postgresql.conf" 2>/dev/null | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "64")

# Extract from conf.d files
CONFD_MAX_CONNECTIONS=$(sudo -u postgres grep -h "max_connections" "${DATA_DIR}/conf.d/"*.conf 2>/dev/null | grep -v "^\s*#" | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "0")
CONFD_MAX_WORKER_PROCESSES=$(sudo -u postgres grep -h "max_worker_processes" "${DATA_DIR}/conf.d/"*.conf 2>/dev/null | grep -v "^\s*#" | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "0")
CONFD_MAX_WAL_SENDERS=$(sudo -u postgres grep -h "max_wal_senders" "${DATA_DIR}/conf.d/"*.conf 2>/dev/null | grep -v "^\s*#" | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "0")
CONFD_MAX_PREPARED_XACTS=$(sudo -u postgres grep -h "max_prepared_transactions" "${DATA_DIR}/conf.d/"*.conf 2>/dev/null | grep -v "^\s*#" | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "0")
CONFD_MAX_LOCKS_PER_XACT=$(sudo -u postgres grep -h "max_locks_per_transaction" "${DATA_DIR}/conf.d/"*.conf 2>/dev/null | grep -v "^\s*#" | tail -1 | sed "s/.*=\s*['\"]*//" | sed "s/['\"].*//" || echo "0")

# Use MAX of both to ensure recovery requirements are met
MAX_CONNECTIONS=$(max_value "$PG_MAX_CONNECTIONS" "$CONFD_MAX_CONNECTIONS")
MAX_WORKER_PROCESSES=$(max_value "$PG_MAX_WORKER_PROCESSES" "$CONFD_MAX_WORKER_PROCESSES")
MAX_WAL_SENDERS=$(max_value "$PG_MAX_WAL_SENDERS" "$CONFD_MAX_WAL_SENDERS")
MAX_PREPARED_XACTS=$(max_value "$PG_MAX_PREPARED_XACTS" "$CONFD_MAX_PREPARED_XACTS")
MAX_LOCKS_PER_XACT=$(max_value "$PG_MAX_LOCKS_PER_XACT" "$CONFD_MAX_LOCKS_PER_XACT")

log "Modifying postgresql.conf for dev branch..."

# Remove problematic lines that would prevent startup
sudo -u postgres sed -i \
    -e '/^include_dir/d' \
    -e '/^pgpodman\./d' \
    -e '/^pg_parquet\./d' \
    -e '/^cron\.use_background_workers/d' \
    -e '/^ssl_ca_file/d' \
    -e '/^archive_mode/d' \
    -e '/^archive_command/d' \
    -e '/^archive_timeout/d' \
    "${DATA_DIR}/postgresql.conf"

sudo -u postgres sed -i \
    "s/shared_preload_libraries = 'pgaudit,pgpodman,anon,pg_squeeze,pg_parquet,pg_cron,pg_stat_statements'/shared_preload_libraries = 'pgaudit,pg_stat_statements'/" \
    "${DATA_DIR}/postgresql.conf"

# Change log destination from syslog to stderr for easier debugging
sudo -u postgres sed -i \
    "s/log_destination = 'syslog'/log_destination = 'stderr'/" \
    "${DATA_DIR}/postgresql.conf"

# Add log_directory since we changed from syslog
if ! grep -q "^log_directory" "${DATA_DIR}/postgresql.conf"; then
    echo "log_directory = 'log'" | sudo -u postgres tee -a "${DATA_DIR}/postgresql.conf" > /dev/null
fi

# Disable archive mode for dev branches
echo "archive_mode = off" | sudo -u postgres tee -a "${DATA_DIR}/postgresql.conf" > /dev/null

# Override network settings for local-only access (append at end to override any earlier settings)
sudo -u postgres tee -a "${DATA_DIR}/postgresql.conf" > /dev/null << EOF

# Branchd overrides for dev branch
port = ${PG_PORT}
listen_addresses = '127.0.0.1'
ssl_cert_file = 'server.crt'
ssl_key_file = 'server.key'

# Recovery-critical parameters from Crunchy Bridge conf.d
# These must be >= primary server values for recovery to succeed
max_connections = ${MAX_CONNECTIONS}
max_worker_processes = ${MAX_WORKER_PROCESSES}
max_wal_senders = ${MAX_WAL_SENDERS}
max_prepared_transactions = ${MAX_PREPARED_XACTS}
max_locks_per_transaction = ${MAX_LOCKS_PER_XACT}

# Performance parameters optimized for dev environment
shared_buffers = '128MB'
huge_pages = try  # Changed from 'on' - VM may not have huge pages configured
EOF

log "postgresql.conf configured"

# Configure pg_hba.conf for local access only
sudo -u postgres tee "${DATA_DIR}/pg_hba.conf" > /dev/null << EOF
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             all                                     peer
host    all             all             127.0.0.1/32            scram-sha-256
host    all             all             ::1/128                 scram-sha-256
EOF

log "PostgreSQL configuration complete"

# 6. Create systemd service for this restore cluster
log "Creating systemd service: ${SERVICE_NAME}"
sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" > /dev/null << EOF
[Unit]
Description=PostgreSQL Restore Cluster from Crunchy Bridge (${DATABASE_NAME})
After=network.target zfs-mount.service
Requires=zfs-mount.service

[Service]
Type=forking
User=postgres
Group=postgres
ExecStart=${PG_BIN}/pg_ctl start -t 3600 -D ${DATA_DIR} -l ${DATA_DIR}/postgresql.log
ExecStop=${PG_BIN}/pg_ctl stop -D ${DATA_DIR} -m fast
ExecReload=${PG_BIN}/pg_ctl reload -D ${DATA_DIR}
KillMode=mixed
KillSignal=SIGINT
TimeoutStartSec=3600
TimeoutStopSec=300
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
log "Systemd service created"

# 7. Start PostgreSQL cluster
log "Starting PostgreSQL cluster..."
sudo systemctl enable "${SERVICE_NAME}" || die "Failed to enable systemd service"
sudo systemctl start "${SERVICE_NAME}" || die "Failed to start systemd service"

# Wait for PostgreSQL to be ready
log "Waiting for PostgreSQL to be ready..."
MAX_RETRIES=60
RETRY_COUNT=0
while [ ${RETRY_COUNT} -lt ${MAX_RETRIES} ]; do
    if sudo -u postgres ${PG_BIN}/pg_isready -p ${PG_PORT} -h 127.0.0.1 >/dev/null 2>&1; then
        log "PostgreSQL is ready and accepting connections"
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    if [ ${RETRY_COUNT} -eq ${MAX_RETRIES} ]; then
        die "PostgreSQL not ready after ${MAX_RETRIES} attempts"
    fi
    log "PostgreSQL not ready, retrying (${RETRY_COUNT}/${MAX_RETRIES})..."
    sleep 1
done

# 8. Clean up pgBackRest config (contains credentials)
log "Cleaning up pgBackRest config..."
if [ -f "${PGBACKREST_CONF}" ]; then
    rm -f "${PGBACKREST_CONF}" || log "Warning: Could not remove pgBackRest config"
    log "pgBackRest config removed"
fi

log "Crunchy Bridge restore completed successfully"
log "Restore cluster running on port ${PG_PORT}"

# Write success marker
echo '__BRANCHD_RESTORE_SUCCESS__' >> "${RESTORE_LOG}"
sync
sleep 0.5

# Remove PID file to signal completion
rm -f "${RESTORE_PID}" || log "Warning: Could not remove PID file"
