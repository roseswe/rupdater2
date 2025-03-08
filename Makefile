# Define the target name and Go source files
# $Id: Makefile,v 1.21 2025/02/21 18:54:09 ralph Exp $

TARGET = rupdater
GOFILES = main.go
BUILD_DIR = build
VERSION=$(shell git describe --tags --abbrev=0 | sed 's/v//g')

# Define cross-compilation settings for Windows and Linux (32-bit)
WINDOWS_ARCH = windows/386
LINUX_ARCH = linux/386

# Global Go flags to remove BuildID, omits the DWARF symbol table, removes the symbol table and debug information.
GOFLAGS := -ldflags "-buildid= -w -s "
##  -buildmode=pie

# Define a variable to hold the date
current_date_mon := $(shell date +%Y%m)
current_date_full := $(shell date +%Y%m%d-%H:%M)

# Default target
all: build-windows build-linux built-darwin

# Create the build directory if it does not exist
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# MacOS, 64bit only, no strip working here
built-darwin:
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) -o $(BUILD_DIR)/$(TARGET)-darwin64  $(GOFILES)


# Windows (32+64-bit) build
build-windows: $(BUILD_DIR)
	goversioninfo -icon main.ico -o resource.syso versioninfo.json
	GOOS=windows GOARCH=386 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET)32.exe $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)32.exe
	@echo "[!] Windows 32-bit executable created: $(BUILD_DIR)/$(TARGET)32.exe"
	GOOS=windows GOARCH=amd64 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET)64.exe $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)64.exe
	@echo "[!] Windows 64-bit executable created: $(BUILD_DIR)/$(TARGET)64.exe"

# Linux (32+64-bit) build
build-linux: $(BUILD_DIR)
	GOOS=linux GOARCH=386 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET)_i686 $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)_i686
	@echo "[!] Linux 32-bit executable created: $(BUILD_DIR)/$(TARGET)_i686"
	GOOS=linux GOARCH=amd64 go build ${GOFLAGS} -o $(BUILD_DIR)/$(TARGET)_amd64 $(GOFILES)
	strip $(BUILD_DIR)/$(TARGET)_amd64
	@echo "[!] Linux 32-bit executable created: $(BUILD_DIR)/$(TARGET)_amd64"

# Clean up the build directory, cleanup misc. stuff
clean:
	rm -rf $(BUILD_DIR)  rupdater2_*.zip
	go telemetry off
	go mod tidy
	@echo "[!] Build directory cleaned. Mod tidied."

# To force re-build
rebuild: clean all
	@echo "[!] Rebuilding project..."

dist: clean all
	file $(BUILD_DIR)/$(TARGET)* > files.txt
	echo "" >> files.txt
	echo "## GLIBC requirements" >> files.txt
	ldd -v $(BUILD_DIR)/$(TARGET)* >> files.txt || true
	upx --best --lzma --force-macos $(BUILD_DIR)/$(TARGET)*
	pandoc README.md -t HTML -o readme.html
	zip -j "rupdater2_$(current_date_mon).zip"  readme.html $(BUILD_DIR)/$(TARGET)* ChangeLog.txt rupdater_example.txt files.txt
	describe "rupdater2_$(current_date_mon).zip" -desc="ROSE SWE updater release2 - downloads new software from the ROSE SWE download page"

changelog:
	gitchangelog > ChangeLog.txt
	git commit -a -s -m "chg: Updated the Changelog (by Makefile) $(current_date_full)"
	cat ChangeLog.txt

# Help message
help:
	@echo "|== Makefile commands =="
	@echo " 	 make build    			- Build the binary"
	@echo " 	 make dist     			- Build and make the ZIP for distribution"
	@echo " 	 make clean    			- Clean up build artifacts"
	@echo " 	 make rebuild  			- Clean and rebuild the project"
	@echo " 	 make changelog			- Generate a changelog from git commits"
	@echo " 	 make all      			- Build for all platforms"
	@echo " 	 make build-windows - Build for Windows"
	@echo " 	 make build-linux   - Build for Linux"
	@echo " 	 make build-darwin  - Build for MacOS"
	@echo " 	 make help          - Show this help message"
