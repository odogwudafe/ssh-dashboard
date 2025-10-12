.PHONY: build clean install test run

BINARY_NAME=ssh-dashboard
INSTALL_PATH=$(HOME)/.local/bin

build:
	@echo "Building..."
	@go build -o ${BINARY_NAME} ./cmd/ssh_dashboard

build-all:
	@echo "Building for multiple platforms..."
	@GOOS=linux GOARCH=amd64 go build -o ${BINARY_NAME}-linux-amd64 ./cmd/ssh_dashboard
	@GOOS=darwin GOARCH=amd64 go build -o ${BINARY_NAME}-darwin-amd64 ./cmd/ssh_dashboard
	@GOOS=darwin GOARCH=arm64 go build -o ${BINARY_NAME}-darwin-arm64 ./cmd/ssh_dashboard
	@GOOS=windows GOARCH=amd64 go build -o ${BINARY_NAME}-windows-amd64.exe ./cmd/ssh_dashboard

clean:
	@echo "Cleaning..."
	@go clean
	@rm -f ${BINARY_NAME}
	@rm -f ${BINARY_NAME}-*

install: build
	@echo "Installing to ${INSTALL_PATH}..."
	@mkdir -p ${INSTALL_PATH}
	@cp ${BINARY_NAME} ${INSTALL_PATH}/
	@chmod +x ${INSTALL_PATH}/${BINARY_NAME}
	@echo "Installed!"
	@echo ""
	@echo "Make sure ${INSTALL_PATH} is in your PATH"

uninstall:
	@echo "Uninstalling from ${INSTALL_PATH}..."
	@rm -f ${INSTALL_PATH}/${BINARY_NAME}
	@echo "Uninstalled!"

run: build
	@./${BINARY_NAME}

test:
	@echo "Running tests..."
	@go test -v ./...

deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy

help:
	@echo "Available targets:"
	@echo "  build      - Build the binary"
	@echo "  build-all  - Build for multiple platforms"
	@echo "  clean      - Remove built binaries"
	@echo "  install    - Install to ${INSTALL_PATH}"
	@echo "  uninstall  - Remove from ${INSTALL_PATH}"
	@echo "  run        - Build and run"
	@echo "  test       - Run tests"
	@echo "  deps       - Download and tidy dependencies"

