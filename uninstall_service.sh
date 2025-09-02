#!/bin/bash
# Uninstallation script for PikaFileService systemd service

set -e

SERVICE_NAME="pikafileservice"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
INSTALL_DIR="/opt/${SERVICE_NAME}"
LOG_DIR="/var/log/${SERVICE_NAME}"

echo "Uninstalling PikaFileService systemd service..."

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (use sudo)"
    exit 1
fi

# Stop and disable service
echo "Stopping and disabling service..."
systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
systemctl disable "${SERVICE_NAME}" 2>/dev/null || true

# Remove service file
echo "Removing service file..."
rm -f "${SERVICE_FILE}"

# Remove installation directory (with confirmation)
if [ -d "${INSTALL_DIR}" ]; then
    read -p "Remove installation directory ${INSTALL_DIR}? [y/N]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf "${INSTALL_DIR}"
        echo "Removed ${INSTALL_DIR}"
    fi
fi

# Remove log directory (with confirmation)
if [ -d "${LOG_DIR}" ]; then
    read -p "Remove log directory ${LOG_DIR}? [y/N]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf "${LOG_DIR}"
        echo "Removed ${LOG_DIR}"
    fi
fi

# Reload systemd
echo "Reloading systemd daemon..."
systemctl daemon-reload

echo "Uninstallation complete!"
