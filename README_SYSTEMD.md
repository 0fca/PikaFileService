# PikaFileService - Systemd Service Setup

PikaFileService is a file synchronization service that can be run as a systemd service with root permissions.

## Files Created

- `pikafileservice.service` - Systemd service definition
- `install_service.sh` - Service installation script
- `uninstall_service.sh` - Service removal script
- `manage_service.sh` - Service management helper
- `Makefile` - Build and service management targets

## Quick Start

### 1. Build and Install Service

```bash
# Build the binary and install as systemd service
make install
```

Or manually:

```bash
# Build the binary
go build -o pikafileservice Main.go

# Install service (requires sudo)
sudo ./install_service.sh
```

### 2. Start the Service

```bash
# Start the service
sudo systemctl start pikafileservice

# Check status
sudo systemctl status pikafileservice
```

### 3. View Logs

```bash
# Follow logs in real-time
sudo journalctl -u pikafileservice -f

# View all logs
sudo journalctl -u pikafileservice
```

## Service Management

### Using the Management Script

```bash
# Start service
./manage_service.sh start

# Stop service
./manage_service.sh stop

# Restart service
./manage_service.sh restart

# Check status
./manage_service.sh status

# Follow logs
./manage_service.sh logs

# Enable auto-start on boot
./manage_service.sh enable

# Disable auto-start
./manage_service.sh disable
```

### Using Make Commands

```bash
# Service management
make service-start
make service-stop
make service-restart
make service-status
make service-logs
make service-enable
make service-disable

# Complete installation
make install

# Complete removal
make uninstall
```

### Using Systemctl Directly

```bash
# Basic service control
sudo systemctl start pikafileservice
sudo systemctl stop pikafileservice
sudo systemctl restart pikafileservice
sudo systemctl status pikafileservice

# Auto-start configuration
sudo systemctl enable pikafileservice
sudo systemctl disable pikafileservice

# View logs
sudo journalctl -u pikafileservice -f
```

## Service Configuration

### Systemd Service Features

- **Runs as root** - Has full filesystem access as required
- **Network dependency** - Waits for network.target before starting
- **Auto-restart** - Automatically restarts if it crashes
- **Resource limits** - Configurable file handle and process limits
- **Security** - Uses systemd security features where possible
- **Logging** - Logs to both systemd journal and application log file

### Installation Locations

- **Binary**: `/opt/pikafileservice/pikafileservice`
- **Config**: `/opt/pikafileservice/config.json`
- **Logs**: `/var/log/pikafileservice/pikafileservice.log`
- **Service**: `/etc/systemd/system/pikafileservice.service`

### Configuration File

The service uses the configuration file at `/opt/pikafileservice/config.json`. Your current configuration will be copied during installation:

```json
{
    "folders": [
        "/home/ofca/sync"
    ],
    "workDir": "/home/ofca/sync",
    "dstPath": "/mnt/public"
}
```

You can modify this file after installation and restart the service to apply changes.

## Troubleshooting

### Check Service Status

```bash
sudo systemctl status pikafileservice
```

### View Recent Logs

```bash
# Last 50 lines
sudo journalctl -u pikafileservice -n 50

# Follow logs
sudo journalctl -u pikafileservice -f

# Application log file
sudo tail -f /var/log/pikafileservice/pikafileservice.log
```

### Debug Mode

To run with debug output, temporarily modify the service:

```bash
sudo systemctl edit pikafileservice
```

Add this content:

```ini
[Service]
ExecStart=
ExecStart=/opt/pikafileservice/pikafileservice -c /opt/pikafileservice/config.json -l /var/log/pikafileservice/pikafileservice.log -d
```

Then restart:

```bash
sudo systemctl daemon-reload
sudo systemctl restart pikafileservice
```

### Common Issues

1. **Permission denied errors**: Ensure the service is running as root
2. **Configuration errors**: Check `/opt/pikafileservice/config.json` syntax
3. **Network issues**: Verify network.target is loaded before service starts
4. **File access issues**: Check that source and destination paths exist and are accessible

## Uninstallation

```bash
# Complete removal
make uninstall

# Or manually
sudo ./uninstall_service.sh
```

This will:
- Stop and disable the service
- Remove the systemd service file
- Optionally remove installation and log directories

## Development

### Building

```bash
# Standard build
make build

# Static binary for deployment
make build-release

# Clean build artifacts
make clean
```

### Testing

```bash
# Run tests
make test

# Run manually for testing
go run Main.go -c config.json -d
```
