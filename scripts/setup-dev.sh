#!/usr/bin/env bash
# Bootstrap a rocco development environment on a bare machine.
#
# Installs the Go toolchain if missing, then the project lint/security tools
# via `make install-tools`, then verifies the workspace builds. Safe to re-run;
# every step is skipped when already satisfied. No prompts — usable from
# agent/CI environments as well as a fresh laptop.
#
# Usage:
#   ./scripts/setup-dev.sh            # install Go if needed + tools + verify
#   GO_VERSION=1.25.5 ./scripts/setup-dev.sh
#   SKIP_TOOLS=1 ./scripts/setup-dev.sh   # Go toolchain only

set -euo pipefail

GO_VERSION="${GO_VERSION:-1.25.5}"   # matches the toolchain directive in go.mod
MIN_GO_MINOR=24                      # go.mod requires go >= 1.24
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

log() { printf '==> %s\n' "$*"; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }

# --- 1. Go toolchain -------------------------------------------------------

have_usable_go() {
    command -v go >/dev/null 2>&1 || return 1
    local minor
    minor="$(go env GOVERSION | sed -E 's/^go1\.([0-9]+).*/\1/')"
    [ "${minor:-0}" -ge "$MIN_GO_MINOR" ]
}

install_go() {
    local os arch tarball url dest
    case "$(uname -s)" in
        Linux)  os=linux ;;
        Darwin) os=darwin ;;
        *) fail "unsupported OS: $(uname -s)" ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64)  arch=amd64 ;;
        aarch64|arm64) arch=arm64 ;;
        *) fail "unsupported arch: $(uname -m)" ;;
    esac

    # Prefer /usr/local when writable (root, containers); else stay in $HOME.
    if [ -w /usr/local ] || [ "$(id -u)" = 0 ]; then
        dest=/usr/local
    else
        dest="$HOME/.local"
        mkdir -p "$dest"
    fi

    tarball="go${GO_VERSION}.${os}-${arch}.tar.gz"
    url="https://go.dev/dl/${tarball}"
    log "installing Go ${GO_VERSION} (${os}/${arch}) to ${dest}/go"

    local tmp
    tmp="$(mktemp -d)"
    # shellcheck disable=SC2064  # expand $tmp now; it is function-local
    trap "rm -rf '$tmp'" EXIT
    curl -fsSL -o "$tmp/$tarball" "$url" || fail "download failed: $url"
    rm -rf "${dest}/go"
    tar -C "$dest" -xzf "$tmp/$tarball"

    export PATH="${dest}/go/bin:$PATH"

    # Persist PATH for future shells without duplicating the line.
    local profile="$HOME/.profile"
    local line="export PATH=\"${dest}/go/bin:\$(go env GOPATH 2>/dev/null || echo \$HOME/go)/bin:\$PATH\""
    if ! grep -qsF "${dest}/go/bin" "$profile" 2>/dev/null; then
        printf '\n# added by rocco scripts/setup-dev.sh\n%s\n' "$line" >> "$profile"
        log "PATH entry added to ${profile} (open a new shell or 'source' it)"
    fi
}

if have_usable_go; then
    log "Go already present: $(go version)"
else
    install_go
    command -v go >/dev/null 2>&1 || fail "Go install completed but 'go' is not on PATH"
    log "installed: $(go version)"
fi

# go install drops binaries in GOPATH/bin — make sure this shell can see them.
GOBIN_DIR="$(go env GOPATH)/bin"
case ":$PATH:" in
    *":${GOBIN_DIR}:"*) ;;
    *) export PATH="${GOBIN_DIR}:$PATH" ;;
esac

# --- 2. Project tools ------------------------------------------------------

if [ "${SKIP_TOOLS:-0}" = 1 ]; then
    log "SKIP_TOOLS=1 — skipping lint/security tool install"
else
    log "installing project tools (golangci-lint, gosec)"
    if command -v make >/dev/null 2>&1; then
        make -C "$REPO_ROOT" install-tools
    else
        # Minimal environments (agent sandboxes) may lack make; mirror the
        # install-tools target directly. Keep versions in sync with Makefile.
        log "make not found — installing tools directly"
        go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.7.2
        go install github.com/securego/gosec/v2/cmd/gosec@latest
    fi
fi

# --- 3. Verify -------------------------------------------------------------

log "verifying workspace builds"
(cd "$REPO_ROOT" && go build ./... && go vet -tags testing ./...)
(cd "$REPO_ROOT/auth0" && go build ./... && go vet -tags testing ./...)

if ! command -v gcc >/dev/null 2>&1 && ! command -v clang >/dev/null 2>&1; then
    log "WARNING: no C compiler found — the race detector needs cgo, so"
    log "  'make test' will fail. Install gcc/clang, or run tests without"
    log "  -race: go test -tags testing ./..."
fi

log "done. Common commands: make test | make lint | make check"
