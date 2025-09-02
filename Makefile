BINARY_NAME=pikafileservice
MAIN_FILE=Main.go
SERVICE_NAME=pikafileservice

.PHONY: build clean install test run build-tools service-install service-uninstall service-start service-stop service-restart service-status service-logs

# Build targets
build:
	go build -o ${BINARY_NAME}

build-release:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o ${BINARY_NAME}

build-tools:
	cd tools && go build -o test_pikacloud test_pikacloud.go

clean:
	go clean
	rm -f ${BINARY_NAME}
	rm -f tools/test_pikacloud

test:
	go test ./...

run:
	go run ${MAIN_FILE}

# Service management targets
service-install: build
	sudo ./install_service.sh

service-uninstall:
	sudo ./uninstall_service.sh

service-start:
	sudo systemctl start ${SERVICE_NAME}

service-stop:
	sudo systemctl stop ${SERVICE_NAME}

service-restart:
	sudo systemctl restart ${SERVICE_NAME}

service-status:
	sudo systemctl status ${SERVICE_NAME} --no-pager

service-logs:
	sudo journalctl -u ${SERVICE_NAME} -f

service-enable:
	sudo systemctl enable ${SERVICE_NAME}

service-disable:
	sudo systemctl disable ${SERVICE_NAME}

# Combined targets
install: build service-install service-enable
	@echo "PikaFileService installed and enabled!"

uninstall: service-stop service-disable service-uninstall
	@echo "PikaFileService uninstalled!"

# Make scripts executable
setup:
	chmod +x install_service.sh
	chmod +x uninstall_service.sh
	chmod +x manage_service.sh
