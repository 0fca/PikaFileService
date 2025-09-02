#!/bin/bash
# Service management script for PikaFileService

SERVICE_NAME="pikafileservice"

show_help() {
    echo "PikaFileService Management Script"
    echo ""
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  start     Start the service"
    echo "  stop      Stop the service"
    echo "  restart   Restart the service"
    echo "  status    Show service status"
    echo "  logs      Show service logs (follow)"
    echo "  enable    Enable auto-start on boot"
    echo "  disable   Disable auto-start on boot"
    echo "  install   Install the service"
    echo "  uninstall Uninstall the service"
    echo "  help      Show this help message"
}

case "$1" in
    start)
        echo "Starting ${SERVICE_NAME}..."
        sudo systemctl start "${SERVICE_NAME}"
        sudo systemctl status "${SERVICE_NAME}" --no-pager
        ;;
    stop)
        echo "Stopping ${SERVICE_NAME}..."
        sudo systemctl stop "${SERVICE_NAME}"
        ;;
    restart)
        echo "Restarting ${SERVICE_NAME}..."
        sudo systemctl restart "${SERVICE_NAME}"
        sudo systemctl status "${SERVICE_NAME}" --no-pager
        ;;
    status)
        sudo systemctl status "${SERVICE_NAME}" --no-pager
        ;;
    logs)
        echo "Following logs for ${SERVICE_NAME} (Ctrl+C to exit)..."
        sudo journalctl -u "${SERVICE_NAME}" -f
        ;;
    enable)
        echo "Enabling ${SERVICE_NAME} for auto-start..."
        sudo systemctl enable "${SERVICE_NAME}"
        ;;
    disable)
        echo "Disabling ${SERVICE_NAME} auto-start..."
        sudo systemctl disable "${SERVICE_NAME}"
        ;;
    install)
        echo "Installing ${SERVICE_NAME} service..."
        sudo ./install_service.sh
        ;;
    uninstall)
        echo "Uninstalling ${SERVICE_NAME} service..."
        sudo ./uninstall_service.sh
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        echo "Invalid command: $1"
        echo ""
        show_help
        exit 1
        ;;
esac
