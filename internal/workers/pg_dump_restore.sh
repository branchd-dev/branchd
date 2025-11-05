#!/bin/bash
# pg_dump/restore Script for Branchd - Async Restore Logic Only
# Prerequisites: All validation, log setup, and PID checks done by Go code
set -euo pipefail

# Configuration from template
readonly CONNECTION_STRING="{{.ConnectionString}}"
readonly PG_VERSION="{{.PgVersion}}"
readonly PG_PORT="{{.PgPort}}"
readonly DATABASE_NAME="{{.DatabaseName}}"
readonly SCHEMA_ONLY="{{.SchemaOnly}}"
readonly PARALLEL_JOBS="{{.ParallelJobs}}"
readonly DUMP_DIR="{{.DumpDir}}"  # e.g., /opt/branchd/restore_2025-01-15_123456

# Paths
readonly RESTORE_LOG_DIR="/var/log/branchd"
readonly RESTORE_LOG="${RESTORE_LOG_DIR}/restore-${DATABASE_NAME}.log"
readonly RESTORE_PID="${RESTORE_LOG_DIR}/restore-${DATABASE_NAME}.pid"
readonly PG_BIN="/usr/lib/postgresql/${PG_VERSION}/bin"

# Helper functions
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1"
}

die() {
    log "ERROR: $1" >&2

    log "Restoring PostgreSQL settings after failure..."
    {{range .ResetSQL}}
    sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "{{.}}" 2>/dev/null || true
    {{end}}
    sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "SELECT pg_reload_conf()" 2>/dev/null || true

    # Clean up dump file
    if [ -f "${DUMP_DIR}" ]; then
        log "Cleaning up dump file..."
        rm -f "${DUMP_DIR}" 2>/dev/null || log "Warning: Could not remove dump file"
    fi

    # Write failure marker
    echo '__BRANCHD_RESTORE_FAILED__' >> "${RESTORE_LOG}"
    sync
    sleep 0.5

    # Remove PID file
    rm -f "${RESTORE_PID}" 2>/dev/null || true
    exit 1
}

# 1. Verify PostgreSQL is running
log "Verifying PostgreSQL is running on port ${PG_PORT}..."
if ! sudo -u postgres ${PG_BIN}/pg_isready -p ${PG_PORT} >/dev/null 2>&1; then
    die "PostgreSQL is not running on port ${PG_PORT}"
fi

# 2. Create target database
log "Creating target database..."
if ! sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "CREATE DATABASE \"${DATABASE_NAME}\"" 2>&1; then
    # Database might already exist
    if ! sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -lqt | cut -d \| -f 1 | grep -qw "${DATABASE_NAME}"; then
        die "Failed to create database ${DATABASE_NAME}"
    fi
    log "Database already exists, continuing..."
fi

# 3. Apply performance optimizations
log "Applying performance optimizations (parallel_jobs=${PARALLEL_JOBS})..."
{{range .TuneSQL}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "{{.}}" 2>&1 || log "Warning: Could not apply tuning parameter"
{{end}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "SELECT pg_reload_conf()" 2>&1 || log "Warning: Could not reload config"

# 4. Create dump file location on zpool (EBS volume)
log "Creating dump file location: ${DUMP_DIR}"
DUMP_PARENT=$(dirname "${DUMP_DIR}")
if ! sudo mkdir -p "${DUMP_PARENT}"; then
    die "Failed to create dump parent directory"
fi
if ! sudo chown postgres:postgres "${DUMP_PARENT}"; then
    die "Failed to set ownership on dump parent directory"
fi

# 5. Start dump (single-threaded, custom format)
log "Starting pg_dump [schema_only=${SCHEMA_ONLY}]..."
DUMP_FLAGS="--format=custom --no-owner --no-acl --no-comments --verbose"

# Add compression based on PostgreSQL version
if [ "${PG_VERSION}" -ge 15 ]; then
    DUMP_FLAGS="${DUMP_FLAGS} --compress=lz4"
    log "Using LZ4 compression (PostgreSQL ${PG_VERSION})"
else
    # PostgreSQL 14 uses numeric compression levels (1-9), 1 = fast gzip
    DUMP_FLAGS="${DUMP_FLAGS} --compress=1"
    log "Using gzip compression level 1 (PostgreSQL ${PG_VERSION})"
fi

if [ "${SCHEMA_ONLY}" = "true" ]; then
    DUMP_FLAGS="${DUMP_FLAGS} --schema-only"
fi

# Run pg_dump to file (custom format)
set +e
sudo -u postgres ${PG_BIN}/pg_dump "${CONNECTION_STRING}" ${DUMP_FLAGS} --file="${DUMP_DIR}" 2>&1
PGDUMP_EXIT=$?
set -e

log "pg_dump completed with exit code: ${PGDUMP_EXIT}"

# Check pg_dump exit code
if [ ${PGDUMP_EXIT} -ne 0 ]; then
    die "pg_dump failed with exit code ${PGDUMP_EXIT}"
fi

# 6. Three-Phase Restore for optimal performance

# Phase 1: Schema - tables, types, sequences, etc.
log "Phase 1/3: Restoring schema..."
SCHEMA_FLAGS="--format=custom --section=pre-data --no-owner --no-acl --verbose"
set +e
sudo -u postgres ${PG_BIN}/pg_restore ${SCHEMA_FLAGS} --dbname="${DATABASE_NAME}" --port=${PG_PORT} "${DUMP_DIR}" 2>&1
SCHEMA_EXIT=$?
set -e

log "Phase 1 completed with exit code: ${SCHEMA_EXIT}"

# Allow warnings (exit code 1) for missing extensions, but fail on fatal errors
if [ ${SCHEMA_EXIT} -gt 1 ]; then
    die "Phase 1 (schema) failed with fatal exit code ${SCHEMA_EXIT}"
fi

# Phase 2: Data (parallel) - bulk data load with NO index overhead
log "Phase 2/3: Loading data (parallel, jobs=${PARALLEL_JOBS})..."
log "Data phase uses MORE parallelism (no index overhead)"
DATA_FLAGS="--format=custom --section=data --jobs=${PARALLEL_JOBS} --no-owner --no-acl --verbose"
set +e
sudo -u postgres ${PG_BIN}/pg_restore ${DATA_FLAGS} --dbname="${DATABASE_NAME}" --port=${PG_PORT} "${DUMP_DIR}" 2>&1
DATA_EXIT=$?
set -e

log "Phase 2 completed with exit code: ${DATA_EXIT}"

if [ ${DATA_EXIT} -gt 1 ]; then
    die "Phase 2 (data) failed with fatal exit code ${DATA_EXIT}"
fi

# Phase 3: Indexes and Constraints (post-data, parallel)
# Build all indexes in parallel from scratch - much faster than incremental
log "Phase 3/3: Building indexes and constraints (parallel, jobs=${PARALLEL_JOBS})..."
log "Indexes built from scratch in parallel - fastest approach for large datasets"
POSTDATA_FLAGS="--format=custom --section=post-data --jobs=${PARALLEL_JOBS} --no-owner --no-acl --verbose"
set +e
sudo -u postgres ${PG_BIN}/pg_restore ${POSTDATA_FLAGS} --dbname="${DATABASE_NAME}" --port=${PG_PORT} "${DUMP_DIR}" 2>&1
POSTDATA_EXIT=$?
set -e

log "Phase 3 completed with exit code: ${POSTDATA_EXIT}"

if [ ${POSTDATA_EXIT} -gt 1 ]; then
    die "Phase 3 (indexes) failed with fatal exit code ${POSTDATA_EXIT}"
fi

# Log overall results
log "THREE-PHASE RESTORE COMPLETED:"
log "  Phase 1 (schema):  exit code ${SCHEMA_EXIT}"
log "  Phase 2 (data):    exit code ${DATA_EXIT} (${PARALLEL_JOBS} parallel jobs)"
log "  Phase 3 (indexes): exit code ${POSTDATA_EXIT} (${PARALLEL_JOBS} parallel jobs)"

# Check for any warnings
if [ ${SCHEMA_EXIT} -ne 0 ] || [ ${DATA_EXIT} -ne 0 ] || [ ${POSTDATA_EXIT} -ne 0 ]; then
    log "Restore completed with warnings (see exit codes above)"
fi

# 7. Verify PostgreSQL is accepting connections
log "Verifying PostgreSQL is accepting connections..."
MAX_RETRIES=10
RETRY_COUNT=0
while [ ${RETRY_COUNT} -lt ${MAX_RETRIES} ]; do
    if sudo -u postgres ${PG_BIN}/pg_isready -p ${PG_PORT} >/dev/null 2>&1; then
        log "PostgreSQL is accepting connections"
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    if [ ${RETRY_COUNT} -eq ${MAX_RETRIES} ]; then
        die "PostgreSQL not accepting connections after restore (${MAX_RETRIES} attempts)"
    fi
    log "PostgreSQL not ready, retrying in 1s (${RETRY_COUNT}/${MAX_RETRIES})..."
    sleep 1
done

# 8. Clean up dump file
log "Cleaning up dump file..."
if [ -f "${DUMP_DIR}" ]; then
    rm -f "${DUMP_DIR}" || log "Warning: Could not remove dump file"
    log "Dump file removed: ${DUMP_DIR}"
fi

# 9. Reset performance optimizations
log "Resetting performance optimizations..."
{{range .ResetSQL}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "{{.}}" 2>&1 || log "Warning: Could not reset parameter"
{{end}}
sudo -u postgres ${PG_BIN}/psql -p ${PG_PORT} -c "SELECT pg_reload_conf()" 2>&1 || log "Warning: Could not reload config"

log "PostgreSQL restore completed successfully [schema_only=${SCHEMA_ONLY}]"

# Write success marker
echo '__BRANCHD_RESTORE_SUCCESS__' >> "${RESTORE_LOG}"
sync
sleep 0.5

# Remove PID file to signal completion
rm -f "${RESTORE_PID}" || log "Warning: Could not remove PID file"
