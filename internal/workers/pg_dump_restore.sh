#!/bin/bash
set -euo pipefail

# pg_dump/restore Script for Branchd
# Dumps from source database and restores to existing PostgreSQL cluster

# Configuration constants
readonly CONNECTION_STRING="{{.ConnectionString}}"
readonly PG_VERSION="{{.PgVersion}}"
readonly PG_PORT="{{.PgPort}}"
readonly DATABASE_NAME="{{.DatabaseName}}"
readonly SCHEMA_ONLY="{{.SchemaOnly}}" # "true" or "false"

# Paths
readonly MOUNTPATH="/opt/branchd/pg${PG_VERSION}/main"
readonly RESTORE_LOG_DIR="/var/log/branchd"
readonly RESTORE_LOG="${RESTORE_LOG_DIR}/restore-${DATABASE_NAME}.log"
readonly RESTORE_PID="${RESTORE_LOG_DIR}/restore-${DATABASE_NAME}.pid"

# PostgreSQL version-specific binary directory
readonly PG_BIN="/usr/lib/postgresql/${PG_VERSION}/bin"

# Functions
log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1"
}

die() {
    log "ERROR: $1" >&2
    exit 1
}

validate_inputs() {
    [[ -n "${CONNECTION_STRING}" ]] || die "CONNECTION_STRING is required"
    [[ -n "${PG_VERSION}" ]] || die "PG_VERSION is required"
    [[ -n "${PG_PORT}" ]] || die "PG_PORT is required"
    [[ -n "${DATABASE_NAME}" ]] || die "DATABASE_NAME is required"
    [[ -n "${SCHEMA_ONLY}" ]] || die "SCHEMA_ONLY is required"

    # Validate PG_PORT is a number
    [[ "${PG_PORT}" =~ ^[0-9]+$ ]] || die "Invalid PG_PORT: ${PG_PORT} (must be a number)"

    # Validate SCHEMA_ONLY
    [[ "${SCHEMA_ONLY}" == "true" || "${SCHEMA_ONLY}" == "false" ]] || die "Invalid SCHEMA_ONLY: ${SCHEMA_ONLY} (must be true or false)"
}

create_log_directory() {
    log "Creating log directory..."

    sudo mkdir -p "${RESTORE_LOG_DIR}" || die "Failed to create log directory"
    sudo chown "$(whoami):$(whoami)" "${RESTORE_LOG_DIR}" || die "Failed to change log directory ownership"
    sudo chmod 755 "${RESTORE_LOG_DIR}" || die "Failed to set log directory permissions"
}

exit_if_restore_in_progress() {
    log "Checking if restore is already in progress for ${DATABASE_NAME}..."

    if [[ -f "${RESTORE_PID}" ]]; then
        local pid
        pid=$(cat "${RESTORE_PID}" 2>/dev/null || echo "")
        if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
            log "Restore process is already running with PID ${pid}"
            log "Monitor progress with: tail -f ${RESTORE_LOG}"
            exit 0
        else
            log "Found stale PID file, will clean up and continue"
            sudo rm -f "${RESTORE_PID}" || log "Warning: Could not remove stale PID file"
        fi
    fi
}

start_async_restore() {
    log "Starting restore process..."

    # Create the restore script that runs asynchronously
    local restore_script="
        set -euo pipefail

        # PostgreSQL version-specific binary directory
        PG_BIN=\"${PG_BIN}\"

        log() {
            echo \"\$(date '+%Y-%m-%d %H:%M:%S') - \$1\"
        }

        die() {
            log \"ERROR: \$1\" >&2

            log \"Restoring PostgreSQL settings after failure...\"
            sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"ALTER SYSTEM RESET fsync\" 2>/dev/null || true
            sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"ALTER SYSTEM RESET maintenance_work_mem\" 2>/dev/null || true
            sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"SELECT pg_reload_conf()\" 2>/dev/null || true

            # Write failure marker directly to log file (bypasses stdout buffering)
            echo '__BRANCHD_RESTORE_FAILED__' >> \"${RESTORE_LOG}\"
            sync
            sleep 0.5
            # Remove PID file on failure
            rm -f \"${RESTORE_PID}\" 2>/dev/null || true
            exit 1
        }

        # 1. Verify PostgreSQL is running on correct port
        log \"Verifying PostgreSQL is running on port ${PG_PORT}...\"
        if ! sudo -u postgres \${PG_BIN}/pg_isready -p ${PG_PORT} >/dev/null 2>&1; then
            die \"PostgreSQL is not running on port ${PG_PORT}\"
        fi

        # 2. Create target database
        log \"Creating target database...\"
        if ! sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"CREATE DATABASE \\\"${DATABASE_NAME}\\\"\" 2>&1; then
            # Database might already exist, check if that's the error
            if ! sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -lqt | cut -d \| -f 1 | grep -qw \"${DATABASE_NAME}\"; then
                die \"Failed to create database ${DATABASE_NAME}\"
            fi
            log \"Database already exists, continuing...\"
        fi

        log \"Applying performance optimizations...\"
        sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"ALTER SYSTEM SET fsync = off\" 2>&1 || log \"Warning: Could not disable fsync\"
        sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"ALTER SYSTEM SET maintenance_work_mem = '1GB'\" 2>&1 || log \"Warning: Could not set maintenance_work_mem\"
        sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"SELECT pg_reload_conf()\" 2>&1 || log \"Warning: Could not reload config\"

        log \"Starting restore [schema_only=${SCHEMA_ONLY}]...\"
        DUMP_FLAGS=\"--no-owner --no-acl --no-comments --verbose --no-sync\"
        if [ \"${SCHEMA_ONLY}\" = \"true\" ]; then
            DUMP_FLAGS=\"\${DUMP_FLAGS} --schema-only\"
        fi

        # Pipe pg_dump directly to psql
        # We use ON_ERROR_STOP=0 to continue past non-fatal errors like missing extensions
        # This allows restores from managed services that have proprietary extensions
        set +e
        sudo -u postgres bash -c \"\${PG_BIN}/pg_dump \\\"${CONNECTION_STRING}\\\" \${DUMP_FLAGS} | \${PG_BIN}/psql -p ${PG_PORT} -d \\\"${DATABASE_NAME}\\\" -v ON_ERROR_STOP=0\" 2>&1
        PIPE_EXIT_CODE=\$?
        set -e

        # Log the exit code for debugging
        log \"Restore completed with exit code \$PIPE_EXIT_CODE\"

        # Only fail on truly fatal errors
        if [ \$PIPE_EXIT_CODE -gt 3 ]; then
            die \"pg_dump | psql piped restore failed with fatal exit code \$PIPE_EXIT_CODE\"
        fi

        if [ \$PIPE_EXIT_CODE -ne 0 ]; then
            log \"Restore completed with warnings\"
        fi

        log \"Verifying PostgreSQL is accepting connections after restore...\"
        MAX_RETRIES=10
        RETRY_COUNT=0
        while [ \$RETRY_COUNT -lt \$MAX_RETRIES ]; do
            if sudo -u postgres \${PG_BIN}/pg_isready -p ${PG_PORT} >/dev/null 2>&1; then
                log \"PostgreSQL is accepting connections\"
                break
            fi
            RETRY_COUNT=\$((RETRY_COUNT + 1))
            if [ \$RETRY_COUNT -eq \$MAX_RETRIES ]; then
                die \"PostgreSQL is not accepting connections after restore (tried \$MAX_RETRIES times)\"
            fi
            log \"PostgreSQL not ready yet, retrying in 1s (attempt \$RETRY_COUNT/\$MAX_RETRIES)...\"
            sleep 1
        done

        log \"Resetting performance optimizations...\"
        sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"ALTER SYSTEM RESET fsync\" 2>&1 || log \"Warning: Could not reset fsync\"
        sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"ALTER SYSTEM RESET maintenance_work_mem\" 2>&1 || log \"Warning: Could not reset maintenance_work_mem\"
        sudo -u postgres \${PG_BIN}/psql -p ${PG_PORT} -c \"SELECT pg_reload_conf()\" 2>&1 || log \"Warning: Could not reload config\"

        log \"PostgreSQL database restore completed successfully [schema_only=${SCHEMA_ONLY}]\"

        # Write success marker directly to log file (bypasses stdout buffering)
        # This ensures the marker is on disk before we remove the PID file
        echo '__BRANCHD_RESTORE_SUCCESS__' >> \"${RESTORE_LOG}\"

        # Force filesystem sync
        sync
        sleep 0.5

        # Remove PID file to signal completion
        # At this point, success marker is guaranteed to be on disk
        rm -f \"${RESTORE_PID}\" || log \"Warning: Could not remove PID file\"
    "

    # Start the restore process in the background and have it write its own PID
    nohup bash <<NESTED_SCRIPT > "${RESTORE_LOG}" 2>&1 &
${restore_script}
NESTED_SCRIPT
    echo $! > "${RESTORE_PID}"

    # Verify PID file was created
    if [[ ! -f "${RESTORE_PID}" ]]; then
        die "Failed to write PID file"
    fi

    log "pg_dump/restore started asynchronously (piped mode - no disk space needed)"
    log "Monitor progress with: tail -f ${RESTORE_LOG}"
}

# Main execution
log "Starting pg_dump/restore for database ${DATABASE_NAME}"

validate_inputs

# Early checks for idempotency
exit_if_restore_in_progress

# Only proceed with setup if not already running
create_log_directory
start_async_restore
