package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBytes writes b to a new file under dir and returns its path.
func writeBytes(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatalf("setup: os.WriteFile(%q): %v", p, err)
	}
	return p
}

const validTOML = `schema_version = 1

[indices]
euribor1y = "SECUENCIA_TIPO_INTERES_1"
`

// TestLoadMissingFileReturnsDefaults: a config that does not exist is not an
// error; Load returns the defaults (CORE-02.2).
func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.toml")

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() on missing file: error = %v, want defaults and no error", err)
	}
	want := Default()
	if got.SchemaVersion != want.SchemaVersion {
		t.Fatalf("Load() SchemaVersion = %d, want %d", got.SchemaVersion, want.SchemaVersion)
	}
	if len(got.Indices) != 0 {
		t.Fatalf("Load() Indices = %v, want empty", got.Indices)
	}
}

// TestLoadParsesFile checks that an existing valid config is read back.
func TestLoadParsesFile(t *testing.T) {
	path := writeBytes(t, t.TempDir(), "config.toml", []byte(validTOML))

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", got.SchemaVersion)
	}
	if got.Indices["euribor1y"] != "SECUENCIA_TIPO_INTERES_1" {
		t.Fatalf("Indices = %v, want the override for euribor1y", got.Indices)
	}
}

// TestLoadCorruptFileErrorsAndNeverOverwrites: a corrupt file is an explicit
// error carrying the path, and the file's bytes are untouched afterwards —
// Load never "repairs" user data (decision 5 of the task).
func TestLoadCorruptFileErrorsAndNeverOverwrites(t *testing.T) {
	corrupt := []byte("schema_version = 1\n[indices\n")
	path := writeBytes(t, t.TempDir(), "config.toml", corrupt)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() on corrupt file: error = nil, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("Load() error %q does not mention the path %q", err, path)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading config after failed Load: %v", readErr)
	}
	if string(after) != string(corrupt) {
		t.Fatalf("corrupt config was modified:\nbefore: %q\nafter:  %q", corrupt, after)
	}
}

// TestLoadUnsupportedSchemaVersion: an unsupported schema_version is rejected
// with a clear message; the file is not touched.
func TestLoadUnsupportedSchemaVersion(t *testing.T) {
	content := []byte("schema_version = 99\n")
	path := writeBytes(t, t.TempDir(), "config.toml", content)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() with schema_version 99: error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("error %q does not mention the unsupported version 99", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading config after failed Load: %v", readErr)
	}
	if string(after) != string(content) {
		t.Fatal("config was modified by a rejected Load")
	}
}

// TestSaveLoadRoundTrip: save then load returns the same SchemaVersion and the
// same Indices map, including an empty map (CORE-02.4).
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.toml") // parent must be created

	for name, cfg := range map[string]Config{
		"with overrides": {
			SchemaVersion: 1,
			Indices: map[string]string{
				"euribor1y": "SECUENCIA_TIPO_INTERES_1",
				"euribor3m": "SECUENCIA_TIPO_INTERES_2",
			},
		},
		"empty indices": {
			SchemaVersion: 1,
			Indices:       map[string]string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := cfg.Save(path); err != nil {
				t.Fatalf("Save() error = %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got.SchemaVersion != cfg.SchemaVersion {
				t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, cfg.SchemaVersion)
			}
			if len(got.Indices) != len(cfg.Indices) {
				t.Fatalf("Indices = %v, want %v", got.Indices, cfg.Indices)
			}
			for k, v := range cfg.Indices {
				if got.Indices[k] != v {
					t.Fatalf("Indices[%q] = %q, want %q", k, got.Indices[k], v)
				}
			}
		})
	}
}

// TestSaveRejectsUnsupportedSchemaVersion: Save never writes a config that
// Load would refuse to read back.
func TestSaveRejectsUnsupportedSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := Config{SchemaVersion: 99, Indices: map[string]string{}}

	if err := cfg.Save(path); err == nil {
		t.Fatal("Save() with schema_version 99: error = nil, want an error")
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatal("Save() with an unsupported version left a file behind")
	}
}
