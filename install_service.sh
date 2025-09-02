#!/bin/bash
# Installation script for PikaFileService systemd service

set -e

SERVICE_NAME="pikafileservice"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
INSTALL_DIR="/opt/${SERVICE_NAME}"
LOG_DIR="/var/log/${SERVICE_NAME}"
BINARY_NAME="${SERVICE_NAME}"

echo "Installing PikaFileService systemd service..."

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (use sudo)"
    exit 1
fi

# Create directories
echo "Creating directories..."
mkdir -p "${INSTALL_DIR}"
mkdir -p "${LOG_DIR}"

# Build the binary if it doesn't exist
if [ ! -f "${BINARY_NAME}" ]; then
    echo "Building PikaFileService binary..."
    /usr/local/go/bin/go build -o "${BINARY_NAME}"
fi

# Copy binary and config
echo "Installing binary and configuration..."
cp "${BINARY_NAME}" "${INSTALL_DIR}/"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

# Copy config if it exists, otherwise create a default one
if [ -f "config.json" ]; then
    cp config.json "${INSTALL_DIR}/"
else
    echo "Creating default config.json..."
    cat > "${INSTALL_DIR}/config.json" << 'EOF'
{
    "folders": [
        "/home/data/source"
    ],
    "dst": "/home/data/destination",
    "workingDirectory": "/tmp/pikafileservice"
}
EOF
fi

# Copy systemd service file
echo "Installing systemd service file..."
cp "${SERVICE_NAME}.service" "${SERVICE_FILE}"

# Set proper permissions
echo "Setting permissions..."
chown -R root:root "${INSTALL_DIR}"
chown -R root:root "${LOG_DIR}"
chmod 644 "${SERVICE_FILE}"

# Reload systemd and enable service
echo "Reloading systemd daemon..."
systemctl daemon-reload

echo "Enabling ${SERVICE_NAME} service..."
systemctl enable "${SERVICE_NAME}"

echo ""
echo "Installation complete!"
echo ""
echo "Service management commands:"
echo "  sudo systemctl start ${SERVICE_NAME}     # Start the service"
echo "  sudo systemctl stop ${SERVICE_NAME}      # Stop the service"
echo "  sudo systemctl restart ${SERVICE_NAME}   # Restart the service"
echo "  sudo systemctl status ${SERVICE_NAME}    # Check service status"
echo "  sudo systemctl enable ${SERVICE_NAME}    # Enable auto-start on boot"
echo "  sudo systemctl disable ${SERVICE_NAME}   # Disable auto-start on boot"
echo ""
echo "Logs can be viewed with:"
echo "  sudo journalctl -u ${SERVICE_NAME} -f    # Follow logs"
echo "  sudo journalctl -u ${SERVICE_NAME}       # View all logs"
echo "  sudo tail -f ${LOG_DIR}/${SERVICE_NAME}.log  # View application logs"
echo ""
echo "Configuration file location: ${INSTALL_DIR}/config.json"
echo "Binary location: ${INSTALL_DIR}/${BINARY_NAME}"
echo "Log directory: ${LOG_DIR}"
