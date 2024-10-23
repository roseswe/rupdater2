# Define the target name and Go source files
# $Id: Makefile,v 1.10 2024/10/18 06:17:50 ralph Exp $

TARGET = rupdater
GOFILES = main.go
BUILD_DIR = build

# Define cross-compilation settings for Windows and Linux (32-bit)
WINDOWS_ARCH = windows/386
LINUX_ARCH = linux/386

# Global Go flags to remove BuildID
GOFLAGS := -ldflags "-buildid="

# Define a variable to hold the date
current_date_mon := $(shell date +%Y%m)
current_date_full := $(shell date +%Y%m%d-%H:%M)

# Default target
all: build-windows build-linux

# Create the build directory if it does not exist
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Windows (32+64-bit) build
build-windows: $(BUILD_DIR)
	goversioninfo -icon main.ico -o resource.syso
	GOOS=windows GOARCH=386 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET).exe $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET).exe
	@echo "[!] Windows 32-bit executable created: $(BUILD_DIR)/$(TARGET).exe"
	GOOS=windows GOARCH=amd64 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET)64.exe $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)64.exe
	@echo "[!] Windows 64-bit executable created: $(BUILD_DIR)/$(TARGET)64.exe"

# Linux (32+64-bit) build
build-linux: $(BUILD_DIR)
	GOOS=linux GOARCH=386 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET) $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)
	@echo "[!] Linux 32-bit executable created: $(BUILD_DIR)/$(TARGET)"
	GOOS=linux GOARCH=amd64 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET)64 $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)64
	@echo "[!] Linux 32-bit executable created: $(BUILD_DIR)/$(TARGET)64"

# Clean up the build directory
clean:
	rm -rf $(BUILD_DIR)
	@echo "[!] Build directory cleaned."

# To force re-build
rebuild: clean all
	@echo "[!] Rebuilding project..."

dist: all
	upx $(BUILD_DIR)/$(TARGET)*
	pandoc README.md -t HTML -o readme.html
	zip -j "rupdater2_$(current_date_mon).zip"  readme.html $(BUILD_DIR)/$(TARGET)* ChangeLog.txt rupdater_example.txt
	describe "rupdater2_$(current_date_mon).zip" -desc="ROSE SWE updater release2 - downloads new software from the ROSE SWE download page"

changelog:
	gitchangelog > ChangeLog.txt
	git commit -a -s -m "chg: Updated Changelog (by Makefile) $(current_date_full)"
	cat ChangeLog.txt

# Help message
help:
	@echo "Makefile commands:"
	@echo "  make build    - Build the binary"
	#@echo "  make test     - Run tests"
	@echo "  make clean    - Clean up build artifacts"
	@echo "  make install   - Install the binary"
	@echo "  make run      - Run the application"
	@echo "  make help     - Show this help message"