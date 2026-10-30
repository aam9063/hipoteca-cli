package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// assertNoTempLeftover fails if dir contains any leftover temp file (the
// ".config.toml.tmp*" pattern writeFileAtomic uses). CORE-02.3: no `.tmp`
// leftovers on success or failure.
func assertNoTempLeftover(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading dir %q: %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("leftover temp file %q in %q", e.Name(), dir)
		}
	}
}

// TestSaveWritesExactBytes: after Save the destination content is exactly the
// marshalled bytes, nothing else (CORE-02.3a).
func TestSaveWritesExactBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := Config{SchemaVersion: 1, Indices: map[string]string{"euribor1y": "SERIE_1"}}

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
		t.Fatalf("file content =\n%q\nwant exactly the marshalled bytes:\n%q", got, want)
	}
	assertNoTempLeftover(t, dir)
}

// TestWriteFileAtomicFailureDirIsFile: the destination's parent is an existing
// regular file, so the directory cannot be created. Works on all three OSes:
// the write fails, the destination stays absent and no temp file remains.
func TestWriteFileAtomicFailureDirIsFile(t *testing.T) {
	dir := t.TempDir()
	notADir := filepath.Join(dir, "blocker")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	path := filepath.Join(notADir, "config.toml") // parent is a file, not a dir

	if err := writeFileAtomic(path, []byte("data"), 0o600); err == nil {
		t.Fatal("writeFileAtomic() error = nil, want an error")
	}
	// El padre es un fichero, así que Stat falla con ENOTDIR en Unix y con
	// ERROR_PATH_NOT_FOUND en Windows. Ninguno de los dos es ErrNotExist para
	// os.IsNotExist, pero en ambos casos el destino no existe: lo que importa es
	// que Stat no lo encuentre.
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("destination exists after a failed write; want it absent")
	}
	assertNoTempLeftover(t, dir)
}

// TestWriteFileAtomicFailureDestIsDir: the destination is an existing
// directory, so the final Rename fails on every OS. The destination must be
// unchanged and no temp file may remain.
func TestWriteFileAtomicFailureDestIsDir(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config.toml")
	if err := os.MkdirAll(dest, 0o700); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := writeFileAtomic(dest, []byte("data"), 0o600); err == nil {
		t.Fatal("writeFileAtomic() over a directory destination: error = nil, want an error")
	}
	info, statErr := os.Stat(dest)
	if statErr != nil || !info.IsDir() {
		t.Fatalf("destination after failed write: stat = %v, %v; want it still a directory", info, statErr)
	}
	assertNoTempLeftover(t, dir)
}

// TestWriteFileAtomicFailureReadOnlyDir keeps old content when the directory is
// not writable. The read-only directory trick does not block file creation on
// Windows, so this case is Unix-only (see the Windows-portable failure tests
// above).
func TestWriteFileAtomicFailureReadOnlyDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory does not prevent file creation on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// chmod the directory after creating "old" inside it.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("setup chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // allow TempDir cleanup

	if err := writeFileAtomic(path, []byte("new"), 0o600); err == nil {
		t.Fatal("writeFileAtomic() in a read-only dir: error = nil, want an error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading destination after failed write: %v", err)
	}
	if string(got) != "old" {
		t.Fatalf("destination = %q after a failed write; want the old content preserved", got)
	}
	assertNoTempLeftover(t, dir)
}

// TestSaveOverwritesExistingContent: a successful Save replaces the previous
// content completely (the atomic rename), never appends or leaves a mix.
func TestSaveOverwritesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	long := []byte(strings.Repeat("x\n", 200))
	if err := os.WriteFile(path, long, 0o600); err != nil {
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
		t.Fatal("destination is not exactly the new content after Save")
	}
	assertNoTempLeftover(t, dir)
}

// TestSavePermissionsUnix: config file 0600, directory 0700 (CORE-02.3,
// CORE-02.5). On Windows the POSIX mode bits do not apply — Go emulates only a
// read-only bit — so nothing is asserted there, by design (decision 3).
func TestSavePermissionsUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows; nothing is asserted there")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.toml")

	if err := Default().Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("config file mode = %o, want 600", got)
	}
	di, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Fatalf("config dir mode = %o, want 700", got)
	}
}
