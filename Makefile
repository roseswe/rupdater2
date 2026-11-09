# Makefile for rupdater -- pure-Go build, no C toolchain.
# $Id: Makefile,v 1.29 2026/11/09 13:00:00 ralph Exp $
# vim:set fileencoding=utf8 fileformat=unix filetype=makefile tabstop=8 noexpandtab:

# ----------------------------------------------------------------------------
# Policy
#  - Pure Go: CGO_ENABLED=0 everywhere. Internal linker, no gcc, no ld
#    warnings, fully static binaries, trivial cross-compilation.
#  - No intermediate 'build' target: it would clash with the build/
#    directory and cause "Circular ... dependency dropped".
#  - goversioninfo writes resource.syso into the SOURCE directory.
#    Any non-Windows recipe MUST `rm -f resource.syso` first, otherwise
#    the PE object is linked into an ELF/Mach-O binary and fails with
#    "unknown relocation type 3". .NOTPARALLEL: prevents a race where
#    build-windows recreates it mid-Linux-build.
#  - Go 1.26+ requires `goversioninfo -64` for the amd64 build: the
#    linker bug fix that separates 386/amd64 relocations means the
#    default 386 .syso will no longer link into a 64-bit binary.
# ----------------------------------------------------------------------------

export CGO_ENABLED = 0

# Avoid -j races on resource.syso. For a 3-target build the loss of
# parallelism is negligible; remove only if you know what you are doing.
.NOTPARALLEL:

# ── variables ────────────────────────────────────────────────
TITLE     = "ROSE SWE Updater - A tool to update ROSE Online client files from the official download page"
INPUT     = README.md
OUTPUT    = readme.html
TEMPLATES = .tmp_header.html .tmp_footer.html

TARGET    = rupdater
BUILD_DIR = build
VERSION  := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')

# Release LDFLAGS: strip DWARF + symbol table, zero build-ID for
# reproducible bytes. Do NOT use in `go test`.
LDFLAGS_RELEASE := -s -w -buildid=

current_date_mon  := $(shell date +%Y%m)
current_date_full := $(shell date +%Y%m%d-%H:%M)

.PHONY: all build-windows build-linux build-darwin docs html pdf \
        changelog dist clean rebuild help

# ── default: build all platforms (docs are a separate target) ─
all: build-windows build-linux build-darwin

# ── build dir ────────────────────────────────────────────────
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# ── Windows (32+64-bit) ──────────────────────────────────────
# goversioninfo writes resource.syso into the source dir.
# -64 flag is REQUIRED on Go 1.26+ for the amd64 build.
build-windows: $(BUILD_DIR)
	goversioninfo -icon=main.ico -manifest=manifest.xml versioninfo.json
	GOOS=windows GOARCH=386   go build -ldflags='$(LDFLAGS_RELEASE)' \
		-o $(BUILD_DIR)/$(TARGET)32.exe .
	@echo "[!] Windows 32-bit executable: $(BUILD_DIR)/$(TARGET)32.exe"
	goversioninfo -64 -icon=main.ico -manifest=manifest.xml versioninfo.json
	GOOS=windows GOARCH=amd64 go build -ldflags='$(LDFLAGS_RELEASE)' \
		-o $(BUILD_DIR)/$(TARGET)64.exe .
	@echo "[!] Windows 64-bit executable: $(BUILD_DIR)/$(TARGET)64.exe"

# ── Linux (32+64-bit) ────────────────────────────────────────
# rm resource.syso FIRST: build-windows leaves a PE object there,
# and linking it into an ELF binary fails with
#   "unknown relocation type 3".
build-linux: $(BUILD_DIR)
	rm -f resource.syso
	GOOS=linux GOARCH=386   go build -ldflags='$(LDFLAGS_RELEASE)' \
		-o $(BUILD_DIR)/$(TARGET)_i686  .
	@echo "[!] Linux 32-bit executable:   $(BUILD_DIR)/$(TARGET)_i686"
	GOOS=linux GOARCH=amd64 go build -ldflags='$(LDFLAGS_RELEASE)' \
		-o $(BUILD_DIR)/$(TARGET)_amd64 .
	@echo "[!] Linux 64-bit executable:   $(BUILD_DIR)/$(TARGET)_amd64"

# ── macOS (amd64) ────────────────────────────────────────────
build-darwin: $(BUILD_DIR)
	rm -f resource.syso
	GOOS=darwin GOARCH=amd64 go build -ldflags='$(LDFLAGS_RELEASE)' \
		-o $(BUILD_DIR)/$(TARGET)-darwin64 .
	@echo "[!] macOS 64-bit executable:   $(BUILD_DIR)/$(TARGET)-darwin64"

# ── docs (separate: not every dev box has pandoc/markdown2pdf) ─
docs: html pdf

# ── clean ────────────────────────────────────────────────────
clean:
	rm -rf $(BUILD_DIR) rupdater2_*.zip $(OUTPUT) \
	       ChangeLog.txt files.txt resource.syso
	go mod tidy
	@echo "[!] Build directory cleaned. Mod tidied."

rebuild: clean all

changelog:
	gitchangelog 2>/dev/null > ChangeLog.txt
	git commit -a -s -m "chg: Build run - updated changelog $(current_date_full)"
	cat ChangeLog.txt

# ── dist (release zip) ───────────────────────────────────────
dist: clean all changelog docs
	file $(BUILD_DIR)/$(TARGET)* > files.txt
	echo "" >> files.txt
	echo "## GLIBC requirements" >> files.txt
	ldd -v $(BUILD_DIR)/$(TARGET)* >> files.txt || true
	@if command -v upx >/dev/null 2>&1; then \
		upx --best --lzma $(BUILD_DIR)/$(TARGET)*; \
	else \
		echo "[!] upx not found, skipping compression"; \
	fi
	zip -j "rupdater2_MP_$(current_date_mon).zip" \
		readme.html $(BUILD_DIR)/$(TARGET)* ChangeLog.txt \
		rupdater_example.txt files.txt README.pdf
	@if command -v describe >/dev/null 2>&1; then \
		describe "rupdater2_MP_$(current_date_mon).zip" \
			-desc="ROSE SWE updater release2"; \
	fi

# ── help ─────────────────────────────────────────────────────
help:
	@echo "|== Makefile commands =="
	@echo "   make               - Build for all platforms (no docs)"
	@echo "   make build-windows - Windows 32/64-bit"
	@echo "   make build-linux   - Linux 32/64-bit"
	@echo "   make build-darwin  - macOS amd64"
	@echo "   make docs          - HTML + PDF from README.md"
	@echo "   make dist          - Full release ZIP"
	@echo "   make clean         - Remove build artifacts"
	@echo "   make rebuild       - clean + all"
	@echo "   make changelog     - gitchangelog -> ChangeLog.txt"
	@echo "   make help          - This message"

# ── html ─────────────────────────────────────────────────────
html: $(INPUT)
	@echo "Style: Polishing $(INPUT) into $(OUTPUT)..."
	@echo '<style>\
		body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif; line-height: 1.6; color: #24292e; max-width: 850px; margin: 40px auto; padding: 0 30px; background-color: #f6f8fa; }\
		.main-card { background: white; padding: 45px; border: 1px solid #d1d5da; border-radius: 6px; box-shadow: 0 8px 24px rgba(149,157,165,0.1); }\
		h1 { color: #0366d6; border-bottom: 1px solid #eaecef; padding-bottom: 0.3em; }\
		h2 { border-bottom: 1px solid #eaecef; padding-bottom: 0.3em; color: #24292e; }\
		code { background-color: rgba(27,31,35,0.05); padding: 0.2em 0.4em; border-radius: 3px; font-family: monospace; font-size: 85%; color: #d73a49; }\
		pre { background-color: #f6f8fa; padding: 16px; border-radius: 6px; overflow: auto; border: 1px solid #dfe2e5; }\
		blockquote { border-left: 4px solid #dfe2e5; color: #6a737d; padding-left: 16px; margin: 0; font-style: italic; }\
		table { border-collapse: collapse; width: 100%; margin: 20px 0; }\
		th, td { border: 1px solid #dfe2e5; padding: 8px 12px; }\
		th { background-color: #f6f8fa; }\
	</style>\
	<div class="main-card">' > .tmp_header.html
	@echo '</div>' > .tmp_footer.html
	pandoc $(INPUT) -s \
		--metadata title=$(TITLE) \
		-H .tmp_header.html \
		-A .tmp_footer.html \
		-o $(OUTPUT)
	@rm -f $(TEMPLATES)
	@echo "Done! Open $(OUTPUT) to see the results."

pdf:
	@echo "Generating PDF"
	markdown2pdf README.md
	