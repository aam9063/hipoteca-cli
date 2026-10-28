#!/usr/bin/env bash
# The single entry point for "is this change healthy?".
# CI and `make check` run exactly this. Local Go is the only requirement.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> gofmt"
unformatted="$(gofmt -l . | grep -v '^\.bin/' || true)"
if [ -n "$unformatted" ]; then
  echo "these files need gofmt:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> go mod tidy (no diff expected)"
cp go.mod go.mod.check.bak
cp go.sum go.sum.check.bak
go mod tidy
if ! diff -q go.mod go.mod.check.bak >/dev/null || ! diff -q go.sum go.sum.check.bak >/dev/null; then
  mv go.mod.check.bak go.mod
  mv go.sum.check.bak go.sum
  echo "go mod tidy changed go.mod/go.sum: commit the tidy result" >&2
  exit 1
fi
rm -f go.mod.check.bak go.sum.check.bak

echo "==> go vet"
go vet ./...

echo "==> staticcheck"
./scripts/tools.sh >/dev/null
./.bin/staticcheck ./...

echo "==> govulncheck"
./.bin/govulncheck ./...

echo "==> go test"
go test ./...

echo "==> ok"
