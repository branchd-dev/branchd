#!/bin/bash
set -eu  # Exit on error and undefined variables, but no pipefail

# Branchd Branch Deletion Script
#
# Deletes a PostgreSQL branch created by create-branch.sh
#
# Flow:
# 1. Extract port from branch's postgresql.conf for UFW cleanup
# 2. Stop and disable systemd service
# 3. Kill any remaining PostgreSQL processes
# 4. Destroy ZFS clone
# 5. Destroy ZFS snapshot (with -R for recursive cleanup)
# 6. Close UFW port
# 7. Output success marker

# Immediate output so we know script started
echo "BRANCH_DELETION_STARTED=true"

# Input parameters
BRANCH_NAME="{{.BranchName}}"
DATASET_NAME="{{.DatasetName}}"

# Configuration
BRANCH_MOUNTPOINT="/opt/branchd/${BRANCH_NAME}"
BRANCH_PGDATA="${BRANCH_MOUNTPOINT}/main"
SERVICE_NAME="branchd-branch-${BRANCH_NAME}"

echo "Deleting branch: ${BRANCH_NAME}"

# Extract port from postgresql.conf for UFW cleanup
PORT=""
if [ -f "${BRANCH_PGDATA}/postgresql.conf" ]; then
    PORT=$(sudo -u postgres grep "^port = " "${BRANCH_PGDATA}/postgresql.conf" 2>/dev/null | awk '{print $3}' || echo "")
    if [ -n "$PORT" ]; then
        echo "Found port ${PORT} from postgresql.conf"
    fi
fi

# Stop and disable systemd service
echo "Stopping systemd service ${SERVICE_NAME}..."
# Check if service file exists (avoid pipefail issues with grep)
if systemctl list-unit-files "${SERVICE_NAME}.service" 2>/dev/null | grep -q .; then
    sudo systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
    sudo systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
    sudo rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    sudo systemctl daemon-reload
    echo "Service stopped and removed"
else
    echo "Service not found, skipping"
fi

# Kill any remaining PostgreSQL processes for this branch
echo "Killing any remaining PostgreSQL processes..."
sudo pkill -f "${BRANCH_PGDATA}" 2>/dev/null || true
sleep 1  # Give processes time to exit

# Stop systemd mount unit (it can auto-remount even with org.openzfs.systemd:ignore=on)
echo "Stopping systemd mount unit for ${BRANCH_MOUNTPOINT}..."
MOUNT_UNIT=$(systemd-escape --path "${BRANCH_MOUNTPOINT}").mount
if systemctl is-active --quiet "${MOUNT_UNIT}" 2>/dev/null; then
    sudo systemctl stop "${MOUNT_UNIT}" 2>/dev/null || true
    echo "Systemd mount unit stopped"
else
    echo "Systemd mount unit not active, skipping"
fi

# Unmount ZFS clone if still mounted
echo "Unmounting ZFS clone tank/${BRANCH_NAME}..."
if sudo zfs list "tank/${BRANCH_NAME}" >/dev/null 2>&1; then
    if sudo zfs get -H -o value mounted "tank/${BRANCH_NAME}" | grep -q "yes"; then
        if sudo zfs unmount "tank/${BRANCH_NAME}" 2>&1; then
            echo "Clone unmounted"
        else
            echo "BRANCHD_ERROR: Failed to unmount ZFS clone (see error above)"
            exit 1
        fi
    else
        echo "Clone already unmounted"
    fi
fi

# Destroy ZFS clone
echo "Destroying ZFS clone tank/${BRANCH_NAME}..."
if sudo zfs list "tank/${BRANCH_NAME}" >/dev/null 2>&1; then
    if sudo zfs destroy "tank/${BRANCH_NAME}" 2>&1; then
        echo "Clone destroyed"
    else
        echo "BRANCHD_ERROR: Failed to destroy ZFS clone (see error above)"
        exit 1
    fi
else
    echo "Clone not found, skipping"
fi

# Destroy ZFS snapshot with recursive flag
echo "Destroying ZFS snapshot ${DATASET_NAME}@${BRANCH_NAME}..."
if sudo zfs list -t snapshot "${DATASET_NAME}@${BRANCH_NAME}" >/dev/null 2>&1; then
    if sudo zfs destroy -R "${DATASET_NAME}@${BRANCH_NAME}" 2>&1; then
        echo "Snapshot destroyed"
    else
        echo "BRANCHD_ERROR: Failed to destroy ZFS snapshot (see error above)"
        exit 1
    fi
else
    echo "Snapshot not found, skipping"
fi

# Remove leftover mountpoint directory (zfs destroy unmounts but leaves directory)
if [ -d "${BRANCH_MOUNTPOINT}" ]; then
    echo "Removing mountpoint directory ${BRANCH_MOUNTPOINT}..."
    sudo rmdir "${BRANCH_MOUNTPOINT}" 2>/dev/null || sudo rm -rf "${BRANCH_MOUNTPOINT}"
    echo "Mountpoint directory removed"
fi

# Close UFW port
if [ -n "$PORT" ]; then
    echo "Closing UFW port ${PORT}..."
    sudo ufw --force delete allow "${PORT}/tcp" 2>/dev/null || true
    echo "Port closed"
else
    echo "No port found, skipping UFW cleanup"
fi

echo "BRANCH_DELETION_SUCCESS=true"
echo "Branch ${BRANCH_NAME} deleted successfully"
