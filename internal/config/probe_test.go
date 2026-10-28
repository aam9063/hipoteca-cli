package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// Probes from the independent review of CORE-02, kept permanently: each one
// pins an observable contract of Load/Save/DataPath that a refactor could
// silently break.

// TestLoadEmptyFileRejected: a 0-byte config file is NOT silently treated as
// defaults; it is an explicit error (a truncated file is exactly the disaster
// the atomic write exists to prevent — decision 5).
func TestLoadEmptyFileRejected(t *testing.T) {
	path := writeBytes(t, t.TempDir(), "config.toml", nil)

	got, err := Load(path)
	if err == nil {
		t.Fatalf("Load() on a 0-byte file: error = nil, got %+v; want an explicit error, not defaults", got)
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("Load() error %q does not mention the missing schema_version", err)
	}
}

// TestSchemaVersionNegativeRejectedOnLoadAndSave: schema_version -1 is
// rejected on both directions, and in neither case is the file touched.
func TestSchemaVersionNegativeRejectedOnLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	content := []byte("schema_version = -1\n")

	// Load rejects and leaves the bytes identical.
	path := writeBytes(t, dir, "config.toml", content)
	if _, err := Load(path); err == nil {
		t.Fatal("Load() with schema_version -1: error = nil, want an error")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading config after failed Load: %v", readErr)
	}
	if string(after) != string(content) {
		t.Fatalf("Load modified the file: before %q, after %q", content, after)
	}

	// Save refuses and leaves an existing file byte-identical.
	saved := []byte("schema_version = 1\n")
	path = writeBytes(t, dir, "save-target.toml", saved)
	bad := Config{SchemaVersion: -1, Indices: map[string]string{}}
	if err := bad.Save(path); err == nil {
		t.Fatal("Save() with schema_version -1: error = nil, want an error")
	}
	after, readErr = os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading config after failed Save: %v", readErr)
	}
	if string(after) != string(saved) {
		t.Fatalf("Save modified the file: before %q, after %q", saved, after)
	}
}

// TestLoadMissingPathCreatesNothing: Load on a missing path must not create
// the file, not even its parent directory (a read must never leave side
// effects on disk).
func TestLoadMissingPathCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-existe", "config.toml")

	if _, err := Load(path); err != nil {
		t.Fatalf("Load() on missing file: error = %v, want defaults and no error", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("Load created the parent directory %q: stat err = %v", filepath.Dir(path), err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Load created the file %q: stat err = %v", path, err)
	}
}

// TestSaveReplacesLongerFileExactly: Save over an existing (longer) file
// replaces it exactly — no residue of the old content after the shorter new
// content.
func TestSaveReplacesLongerFileExactly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(strings.Repeat("# viejo\n", 500)), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	cfg := Default()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading saved config: %v", err)
	}
	want, err := marshal(cfg)
	if err != nil {
		t.Fatalf("marshal in test: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("file after Save is not exactly the new content (len %d, want %d)", len(got), len(want))
	}
	assertNoTempLeftover(t, dir)
}

// TestSaveFailsWhenDestOpenWithoutShareDelete (Windows only): a destination
// held open WITHOUT FILE_SHARE_DELETE makes the final rename fail; Save then
// returns an error, the old content stays byte-identical and no .tmp is left
// behind. Go's os.OpenFile passes FILE_SHARE_DELETE on Windows, so a raw
// syscall.Open (which does not) is used to hold the destination. Skipped
// elsewhere: the sharing-mode concept is Windows-specific.
func TestSaveFailsWhenDestOpenWithoutShareDelete(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("FILE_SHARE_DELETE semantics are Windows-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	old := []byte("schema_version = 1\n")
	if err := os.WriteFile(path, old, 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// syscall.Open on Windows opens without FILE_SHARE_DELETE (unlike
	// os.OpenFile). Compile-safe on every OS; only run on Windows.
	fd, err := syscall.Open(path, syscall.O_RDWR, 0)
	if err != nil {
		t.Fatalf("setup: syscall.Open: %v", err)
	}
	if err := os.NewFile(uintptr(fd), path).Close(); err != nil {
		t.Fatalf("setup: closing the held handle: %v", err)
	}

	// Re-open and keep it open while Save runs.
	fd, err = syscall.Open(path, syscall.O_RDWR, 0)
	if err != nil {
		t.Fatalf("setup: syscall.Open (held): %v", err)
	}
	held := os.NewFile(uintptr(fd), path)
	defer held.Close()

	if err := Default().Save(path); err == nil {
		t.Fatal("Save() with the destination open without FILE_SHARE_DELETE: error = nil, want an error")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading destination after failed Save: %v", readErr)
	}
	if string(after) != string(old) {
		t.Fatalf("destination changed after the failed Save: before %q, after %q", old, after)
	}
	assertNoTempLeftover(t, dir)
}

// TestDataPathEmptyBaseErrors: an empty base (empty LocalAppData on Windows;
// empty HOME and XDG_DATA_HOME on Unix) is an error, never a CWD-relative
// path like "hipoteca-cli\hipoteca.db" with a nil error.
func TestDataPathEmptyBaseErrors(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	switch runtime.GOOS {
	case "windows":
		t.Setenv("LocalAppData", "")
	default:
		t.Setenv("HOME", "")
	}
	got, err := DataPath()
	if err == nil {
		t.Fatalf("DataPath() = %q with an empty base; want an error, never a relative path", got)
	}
	if got != "" {
		t.Fatalf("DataPath() returned %q alongside an error; want an empty path", got)
	}
}
