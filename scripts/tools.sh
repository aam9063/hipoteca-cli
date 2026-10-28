#!/usr/bin/env bash
# Installs the analysis tools the check needs into ./.bin so `make check` works without
# polluting the user's GOPATH. Idempotent: skips what is already there.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT/.bin"
mkdir -p "$BIN"

install_if_missing() {
  local name="$1" pkg="$2"
  if [ -x "$BIN/$name" ]; then
    return 0
  fi
  echo "installing $name"
  GOBIN="$BIN" go install "$pkg"
}

install_if_missing staticcheck "honnef.co/go/tools/cmd/staticcheck@2025.1.1"
install_if_missing govulncheck "golang.org/x/vuln/cmd/govulncheck@v1.9.0"
