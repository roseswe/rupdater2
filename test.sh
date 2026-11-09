#!/usr/bin/env bash
## @(#) $Id:$

set -euo pipefail

# Pure Go program -- CGO is not needed. Disabling it uses the
# internal linker (no gcc, no ld, no .note.GNU-stack warning)
# and removes any dependency on the host C toolchain.
export CGO_ENABLED=0

# Keep the ld-flag workaround in place for the pure-Go builds
# where GOFLAGS still reaches a real linker on older setups.
export GOFLAGS='-ldflags=-extldflags=-Wl,--no-warn-execstack'

go vet   ./...
go build ./...
go test -v ./...
