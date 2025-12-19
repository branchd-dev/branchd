#!/bin/bash
# pg_dump/restore Script for Branchd - Each restore is a separate PostgreSQL cluster
set -euo pipefail

# Configuration from template
readonly CONNECTION_STRING="{{.ConnectionString}}"
readonly PG_VERSION="{{.PgVersion}}"
readonly PG_PORT="{{.PgPort}}"
readonly DATABASE_NAME="{{.DatabaseName}}"
readonly SCHEMA_ONLY="{{.SchemaOnly}}"
readonly PARALLEL_JOBS="{{.ParallelJobs}}"
readonly DUMP_FILE="{{.DumpDir}}"      # e.g., /opt/branchd/restore_20250915120000/dump.pgdump
readonly DATA_DIR="{{.DataDir}}"       # e.g., /opt/branchd/restore_20250915120000/data

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
    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        log "Stopping PostgreSQL service..."
        sudo systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
    fi

    # Remove systemd service
    if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
        log "Removing systemd service..."
        sudo systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
        sudo rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
        sudo systemctl daemon-reload
    fi

    # Clean up dump file
    if [ -f "${DUMP_FILE}" ]; then
        log "Cleaning up dump file..."
        rm -f "${DUMP_FILE}" 2>/dev/null || log "Warning: Could not remove dump file"
    fi

    # Destroy ZFS dataset if it was created
    if sudo zfs list "${ZFS_DATASET}" >/dev/null 2>&1; then
        log "Destroying ZFS dataset..."
        sudo zfs destroy -r "${ZFS_DATASET}" 2>/dev/null || log "Warning: Could not destroy ZFS dataset"
    fi

    # Write failure marker
    echo '__BRANCHD_RESTORE_FAILED__' >> "${RESTORE_LOG}"
    sync
    sleep 0.5

    # Remove PID file
    rm -f "${RESTORE_PID}" 2>/dev/null || true
    exit 1
}

log "Starting restore: ${DATABASE_NAME}"
log "PostgreSQL version: ${PG_VERSION}, Port: ${PG_PORT}"
log "Data directory: ${DATA_DIR}"
log "Dump file: ${DUMP_FILE}"

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

# 3. Initialize PostgreSQL cluster with initdb
log "Initializing PostgreSQL cluster with initdb..."
sudo -u postgres ${PG_BIN}/initdb -D "${DATA_DIR}" \
    --encoding=UTF8 \
    --locale=C.UTF-8 \
    --data-checksums \
    || die "Failed to initialize PostgreSQL cluster with initdb"
log "PostgreSQL cluster initialized successfully"

# 4. Configure PostgreSQL
log "Configuring PostgreSQL..."

# Copy TLS certificates (shared across all clusters)
sudo -u postgres cp /etc/postgresql-common/ssl/server.crt "${DATA_DIR}/"
sudo -u postgres cp /etc/postgresql-common/ssl/server.key "${DATA_DIR}/"
# Fix permissions on server.key (PostgreSQL requires 0600)
sudo -u postgres chmod 0600 "${DATA_DIR}/server.key"
sudo -u postgres chmod 0644 "${DATA_DIR}/server.crt"

# Update postgresql.conf
sudo -u postgres tee "${DATA_DIR}/postgresql.conf" > /dev/null << EOF
# Basic settings
port = ${PG_PORT}
listen_addresses = '127.0.0.1'
max_connections = 100
shared_buffers = 128MB
work_mem = 4MB
maintenance_work_mem = 64MB

# WAL settings
wal_level = replica
max_wal_size = 1GB
min_wal_size = 80MB

# Logging
logging_collector = on
log_directory = 'log'
log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log'
log_rotation_age = 1d
log_rotation_size = 100MB
log_line_prefix = '%m [%p] %u@%d '
log_timezone = 'UTC'

# Locale
lc_messages = 'C.UTF-8'
lc_monetary = 'C.UTF-8'
lc_numeric = 'C.UTF-8'
lc_time = 'C.UTF-8'

# TLS/SSL
ssl = on
ssl_cert_file = 'server.crt'
ssl_key_file = 'server.key'

# Default locale
datestyle = 'iso, mdy'
timezone = 'UTC'
default_text_search_config = 'pg_catalog.english'
EOF

# Configure pg_hba.conf for local access only
sudo -u postgres tee "${DATA_DIR}/pg_hba.conf" > /dev/null << EOF
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             all                                     peer
host    all             all             127.0.0.1/32            scram-sha-256
host    all             all             ::1/128                 scram-sha-256
EOF

log "PostgreSQL configuration complete"

# 5. Create systemd service for this restore cluster
log "Creating systemd service: ${SERVICE_NAME}"
sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" > /dev/null << EOF
[Unit]
Description=PostgreSQL Restore Cluster (${DATABASE_NAME})
After=network.target zfs-mount.service
Requires=zfs-mount.service

[Service]
Type=forking
User=postgres
Group=postgres
ExecStart=${PG_BIN}/pg_ctl start -D ${DATA_DIR} -l ${DATA_DIR}/postgresql.log
ExecStop=${PG_BIN}/pg_ctl stop -D ${DATA_DIR} -m fast
ExecReload=${PG_BIN}/pg_ctl reload -D ${DATA_DIR}
KillMode=mixed
KillSignal=SIGINT
TimeoutStartSec=300
TimeoutStopSec=300
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
log "Systemd service created"

# 6. Start PostgreSQL cluster
log "Starting PostgreSQL cluster..."
sudo systemctl enable "${SERVICE_NAME}"
sudo systemctl start "${SERVICE_NAME}"

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

# 7. Apply performance optimizations for restore
log "Applying performance optimizations (parallel_jobs=${PARALLEL_JOBS})..."
{{range .TuneSQL}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "{{.}}" 2>&1 || log "Warning: Could not apply tuning parameter"
{{end}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "SELECT pg_reload_conf()" 2>&1 || log "Warning: Could not reload config"

# 8. Start pg_dump from source database
log "Starting pg_dump [schema_only=${SCHEMA_ONLY}]..."
DUMP_FLAGS="--format=custom --no-owner --no-acl --no-comments --verbose"

# Add compression based on PostgreSQL version
if [ "${PG_VERSION}" -ge 15 ]; then
    DUMP_FLAGS="${DUMP_FLAGS} --compress=lz4"
    log "Using LZ4 compression (PostgreSQL ${PG_VERSION})"
else
    DUMP_FLAGS="${DUMP_FLAGS} --compress=1"
    log "Using gzip compression level 1 (PostgreSQL ${PG_VERSION})"
fi

if [ "${SCHEMA_ONLY}" = "true" ]; then
    DUMP_FLAGS="${DUMP_FLAGS} --schema-only"
fi

# Run pg_dump to file
set +e
sudo -u postgres ${PG_BIN}/pg_dump "${CONNECTION_STRING}" ${DUMP_FLAGS} --file="${DUMP_FILE}" 2>&1
PGDUMP_EXIT=$?
set -e

log "pg_dump completed with exit code: ${PGDUMP_EXIT}"

if [ ${PGDUMP_EXIT} -ne 0 ]; then
    die "pg_dump failed with exit code ${PGDUMP_EXIT}"
fi

# 9. Create target database (same name as source)
log "Creating target database: {{.SourceDatabaseName}}"
sudo -u postgres ${PG_BIN}/createdb -p ${PG_PORT} "{{.SourceDatabaseName}}" || log "Database may already exist"

# 10. Three-Phase Restore

# Phase 1: Schema
log "Phase 1/3: Restoring schema..."
SCHEMA_FLAGS="--format=custom --section=pre-data --no-owner --no-acl --verbose"
set +e
sudo -u postgres ${PG_BIN}/pg_restore ${SCHEMA_FLAGS} --dbname="{{.SourceDatabaseName}}" --port=${PG_PORT} "${DUMP_FILE}" 2>&1
SCHEMA_EXIT=$?
set -e

log "Phase 1 completed with exit code: ${SCHEMA_EXIT}"

if [ ${SCHEMA_EXIT} -gt 1 ]; then
    die "Phase 1 (schema) failed with fatal exit code ${SCHEMA_EXIT}"
fi

# Phase 2: Data (parallel)
log "Phase 2/3: Loading data (parallel, jobs=${PARALLEL_JOBS})..."
DATA_FLAGS="--format=custom --section=data --jobs=${PARALLEL_JOBS} --no-owner --no-acl --verbose"
set +e
sudo -u postgres ${PG_BIN}/pg_restore ${DATA_FLAGS} --dbname="{{.SourceDatabaseName}}" --port=${PG_PORT} "${DUMP_FILE}" 2>&1
DATA_EXIT=$?
set -e

log "Phase 2 completed with exit code: ${DATA_EXIT}"

if [ ${DATA_EXIT} -gt 1 ]; then
    die "Phase 2 (data) failed with fatal exit code ${DATA_EXIT}"
fi

# Phase 3: Indexes and Constraints (parallel)
log "Phase 3/3: Building indexes and constraints (parallel, jobs=${PARALLEL_JOBS})..."
POSTDATA_FLAGS="--format=custom --section=post-data --jobs=${PARALLEL_JOBS} --no-owner --no-acl --verbose"
set +e
sudo -u postgres ${PG_BIN}/pg_restore ${POSTDATA_FLAGS} --dbname="{{.SourceDatabaseName}}" --port=${PG_PORT} "${DUMP_FILE}" 2>&1
POSTDATA_EXIT=$?
set -e

log "Phase 3 completed with exit code: ${POSTDATA_EXIT}"

if [ ${POSTDATA_EXIT} -gt 1 ]; then
    die "Phase 3 (indexes) failed with fatal exit code ${POSTDATA_EXIT}"
fi

log "THREE-PHASE RESTORE COMPLETED:"
log "  Phase 1 (schema):  exit code ${SCHEMA_EXIT}"
log "  Phase 2 (data):    exit code ${DATA_EXIT} (${PARALLEL_JOBS} parallel jobs)"
log "  Phase 3 (indexes): exit code ${POSTDATA_EXIT} (${PARALLEL_JOBS} parallel jobs)"

# 10. Clean up dump file
log "Cleaning up dump file..."
if [ -f "${DUMP_FILE}" ]; then
    rm -f "${DUMP_FILE}" || log "Warning: Could not remove dump file"
    log "Dump file removed"
fi

# 11. Reset performance optimizations
log "Resetting performance optimizations..."
{{range .ResetSQL}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "{{.}}" 2>&1 || log "Warning: Could not reset parameter"
{{end}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "SELECT pg_reload_conf()" 2>&1 || log "Warning: Could not reload config"

log "PostgreSQL restore completed successfully [schema_only=${SCHEMA_ONLY}]"
log "Restore cluster running on port ${PG_PORT}"

# Write success marker
echo '__BRANCHD_RESTORE_SUCCESS__' >> "${RESTORE_LOG}"
sync
sleep 0.5

# Remove PID file to signal completion
rm -f "${RESTORE_PID}" || log "Warning: Could not remove PID file"
