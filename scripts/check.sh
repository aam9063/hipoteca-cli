#!/usr/bin/env bash
# The single entry point for "is this change healthy?".
# CI and `make check` run exactly this. Local Go is the only requirement.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> gofmt"
# .bin/ only ever holds installed tool binaries (scripts/tools.sh), never .go files,
# so nothing there can be flagged.
unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ]; then
  echo "these files need gofmt:" >&2
  echo "$unformatted" >&2
  exit 1
fi

echo "==> go mod tidy (no diff expected)"
cp go.mod go.mod.check.bak
cp go.sum go.sum.check.bak
# While the backups exist, any way out of the script restores the committed module
# files: a failed or interrupted `go mod tidy` may leave go.mod/go.sum mutated.
restore_module_files() {
  [ -f go.mod.check.bak ] || return 0
  mv -f go.mod.check.bak go.mod
  mv -f go.sum.check.bak go.sum
}
trap restore_module_files EXIT

if ! go mod tidy; then
  echo "go mod tidy failed: go.mod/go.sum restored to the committed state" >&2
  exit 1
fi
if ! diff -q go.mod go.mod.check.bak >/dev/null || ! diff -q go.sum go.sum.check.bak >/dev/null; then
  echo "go mod tidy changed go.mod/go.sum: commit the tidy result (go.mod/go.sum restored)" >&2
  exit 1
fi
rm -f go.mod.check.bak go.sum.check.bak
trap - EXIT

echo "==> go vet"
go vet ./...

echo "==> staticcheck"
bash "$ROOT/scripts/tools.sh" >/dev/null
./.bin/staticcheck ./...

echo "==> govulncheck"
./.bin/govulncheck ./...

echo "==> go test"
go test ./...

echo "==> ok"
