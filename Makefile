# Define the target name and Go source files
# $Id: Makefile,v 1.2 2024/10/03 22:07:47 ralph Exp $

TARGET = rupdater
GOFILES = main.go
BUILD_DIR = build

# Define cross-compilation settings for Windows and Linux (32-bit)
WINDOWS_ARCH = windows/386
LINUX_ARCH = linux/386

# Define a variable to hold the date
current_date_mon := $(shell date +%Y%m)
current_date_full := $(shell date +%Y%m%d-%H:%M)

# Default target
all: build-windows build-linux

# Create the build directory if it does not exist
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Windows (32-bit) build
build-windows: $(BUILD_DIR)
	GOOS=windows GOARCH=386 go build -o $(BUILD_DIR)/$(TARGET).exe $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET).exe
	@echo "[!] Windows 32-bit executable created: $(BUILD_DIR)/$(TARGET).exe"
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(TARGET)64.exe $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)64.exe
	@echo "[!] Windows 64-bit executable created: $(BUILD_DIR)/$(TARGET)64.exe"

# Linux (32-bit) build
build-linux: $(BUILD_DIR)
	GOOS=linux GOARCH=386 go build -o $(BUILD_DIR)/$(TARGET) $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)
	@echo "[!] Linux 32-bit executable created: $(BUILD_DIR)/$(TARGET)"

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
	zip -j "rupdater2_$(current_date_mon)"  readme.html $(BUILD_DIR)/$(TARGET)* ChangeLog.txt

changelog:
	gitchangelog > ChangeLog.txt
	git commit -a -s -m "chg: Updated Changelog (by Makefile) $(current_date_full)"
	cat ChangeLog.txt
