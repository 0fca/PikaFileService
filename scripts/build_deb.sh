#!/bin/bash
# Build a .deb package for PikaFileService
# Usage: ./scripts/build_deb.sh [version] [arch]
#   version  - package version (default: 1.0.0)
#   arch     - target architecture (default: amd64)

set -euo pipefail

VERSION="${1:-1.0.0}"
ARCH="${2:-amd64}"
PKG_NAME="pikafileservice"
PKG_DIR="build/${PKG_NAME}_${VERSION}_${ARCH}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "${SCRIPT_DIR}")"

echo "==> Building ${PKG_NAME} ${VERSION} (${ARCH}) .deb package"

# Ensure the binary exists
if [ ! -f "${PROJECT_DIR}/${PKG_NAME}" ]; then
    echo "ERROR: Binary '${PKG_NAME}' not found. Run 'make build-release' first."
    exit 1
fi

# Clean previous build
rm -rf "${PROJECT_DIR}/${PKG_DIR}"

# -- Create directory structure matching the systemd service paths --
mkdir -p "${PROJECT_DIR}/${PKG_DIR}/DEBIAN"
mkdir -p "${PROJECT_DIR}/${PKG_DIR}/opt/${PKG_NAME}"
mkdir -p "${PROJECT_DIR}/${PKG_DIR}/var/log/${PKG_NAME}"
mkdir -p "${PROJECT_DIR}/${PKG_DIR}/lib/systemd/system"

# -- Copy binary --
cp "${PROJECT_DIR}/${PKG_NAME}" "${PROJECT_DIR}/${PKG_DIR}/opt/${PKG_NAME}/${PKG_NAME}"
chmod 755 "${PROJECT_DIR}/${PKG_DIR}/opt/${PKG_NAME}/${PKG_NAME}"

# -- Copy systemd service file --
cp "${PROJECT_DIR}/${PKG_NAME}.service" "${PROJECT_DIR}/${PKG_DIR}/lib/systemd/system/${PKG_NAME}.service"
chmod 644 "${PROJECT_DIR}/${PKG_DIR}/lib/systemd/system/${PKG_NAME}.service"

# -- Ship a default config (placeholder values) --
cat > "${PROJECT_DIR}/${PKG_DIR}/opt/${PKG_NAME}/config.json" << 'DEFAULTCFG'
{
    "folders": [
        "/home/data/source"
    ],
    "workDir": "/home/data/source",
    "dstPath": "/home/data/destination"
}
DEFAULTCFG
chmod 640 "${PROJECT_DIR}/${PKG_DIR}/opt/${PKG_NAME}/config.json"

# -- DEBIAN/control --
INSTALLED_SIZE=$(du -sk "${PROJECT_DIR}/${PKG_DIR}" | awk '{print $1}')
cat > "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/control" << EOF
Package: ${PKG_NAME}
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: ${ARCH}
Installed-Size: ${INSTALLED_SIZE}
Maintainer: Łukasz Bownik <lukasz@lukas-bownik.net>
Description: PikaFileService - File Synchronization Service
 A file watcher and synchronization daemon that monitors directories
 for changes and replicates them to a destination path, optionally
 uploading files to a PikaCloud (PikaCore) storage backend.
Homepage: https://github.com/0fca/PikaFileService
EOF

# -- DEBIAN/conffiles (mark config so dpkg won't overwrite user edits on upgrade) --
cat > "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/conffiles" << EOF
/opt/${PKG_NAME}/config.json
EOF

# -- DEBIAN/preinst --
cat > "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/preinst" << 'EOF'
#!/bin/bash
set -e
# Stop the service before upgrade if it's running
if systemctl is-active --quiet pikafileservice 2>/dev/null; then
    echo "Stopping pikafileservice before upgrade..."
    systemctl stop pikafileservice
fi
exit 0
EOF
chmod 755 "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/preinst"

# -- DEBIAN/postinst --
cat > "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/postinst" << 'EOF'
#!/bin/bash
set -e

# Create log directory (in case it was removed)
mkdir -p /var/log/pikafileservice
chown root:root /var/log/pikafileservice

# Reload systemd to pick up the service file
systemctl daemon-reload

# Enable the service (but don't start — user must configure first)
systemctl enable pikafileservice

echo ""
echo "╔═══════════════════════════════════════════════════════╗"
echo "║  PikaFileService installed successfully!              ║"
echo "╠═══════════════════════════════════════════════════════╣"
echo "║                                                       ║"
echo "║  1. Edit the configuration:                           ║"
echo "║     sudo nano /opt/pikafileservice/config.json        ║"
echo "║                                                       ║"
echo "║  2. Start the service:                                ║"
echo "║     sudo systemctl start pikafileservice              ║"
echo "║                                                       ║"
echo "║  3. Check status:                                     ║"
echo "║     sudo systemctl status pikafileservice             ║"
echo "║                                                       ║"
echo "║  Logs: sudo journalctl -u pikafileservice -f          ║"
echo "╚═══════════════════════════════════════════════════════╝"
echo ""

exit 0
EOF
chmod 755 "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/postinst"

# -- DEBIAN/prerm --
cat > "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/prerm" << 'EOF'
#!/bin/bash
set -e
# Stop and disable the service before removal
if systemctl is-active --quiet pikafileservice 2>/dev/null; then
    echo "Stopping pikafileservice..."
    systemctl stop pikafileservice
fi
systemctl disable pikafileservice 2>/dev/null || true
exit 0
EOF
chmod 755 "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/prerm"

# -- DEBIAN/postrm --
cat > "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/postrm" << 'EOF'
#!/bin/bash
set -e
# Reload systemd after service file removal
systemctl daemon-reload

# On purge, remove config and logs
if [ "$1" = "purge" ]; then
    rm -rf /opt/pikafileservice
    rm -rf /var/log/pikafileservice
fi
exit 0
EOF
chmod 755 "${PROJECT_DIR}/${PKG_DIR}/DEBIAN/postrm"

# -- Build the .deb --
DEB_OUTPUT="${PROJECT_DIR}/build/${PKG_NAME}_${VERSION}_${ARCH}.deb"
dpkg-deb --build --root-owner-group "${PROJECT_DIR}/${PKG_DIR}" "${DEB_OUTPUT}"

echo ""
echo "==> Package built: ${DEB_OUTPUT}"
echo "    Install with:  sudo dpkg -i ${DEB_OUTPUT}"
echo ""
