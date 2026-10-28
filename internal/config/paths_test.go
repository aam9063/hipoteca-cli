package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

// tempDirFor returns an absolute temp directory for the XDG_* variables. It is
// removed by t.TempDir cleanup; nothing is left in the worktree.
func tempDirFor(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// wantHomeSubdir returns the platform's own answer for base under a temp
// $HOME, so each assertion names the expected default explicitly. Only
// meaningful off Windows (HOME does not drive these paths there).
func wantHomeSubdir(goos, home string) string {
	switch goos {
	case "darwin":
		// macOS: os.UserConfigDir/os.UserCacheDir answer with ~/Library/...,
		// and our data fallback matches (Application Support).
		return filepath.Join(home, "Library", "Application Support")
	default:
		// Linux and the rest of Unix: XDG defaults.
		return filepath.Join(home, ".config")
	}
}

// TestConfigPathRespectsXDGConfigHome checks that an absolute
// XDG_CONFIG_HOME wins on EVERY non-Windows OS, including macOS where
// os.UserConfigDir alone would ignore the variable (our precedence is
// explicit, see resolveBase). On Windows XDG does not apply:
// see TestConfigPathWindows.
func TestConfigPathRespectsXDGConfigHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CONFIG_HOME does not apply on Windows; see TestConfigPathWindows")
	}
	base := tempDirFor(t)
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("HOME", tempDirFor(t)) // must not influence the result

	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	want := filepath.Join(base, "hipoteca-cli", "config.toml")
	if got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}

// TestConfigPathFallbackToHome checks the platform-specific default when
// XDG_CONFIG_HOME is empty: ~/.config on Linux and the rest of Unix,
// ~/Library/Application Support on macOS (runtime.GOOS-guarded: the expected
// value is a per-OS fact). On Windows the equivalent is %AppData%, asserted in
// TestConfigPathWindows.
func TestConfigPathFallbackToHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-based fallback does not apply on Windows; see TestConfigPathWindows")
	}
	home := tempDirFor(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	want := filepath.Join(wantHomeSubdir(runtime.GOOS, home), "hipoteca-cli", "config.toml")
	if got != want {
		t.Fatalf("ConfigPath() on %s = %q, want %q", runtime.GOOS, got, want)
	}
}

// TestConfigPathWindows checks the %AppData% resolution on Windows.
func TestConfigPathWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only meaningful on Windows")
	}
	base := tempDirFor(t)
	t.Setenv("AppData", base)

	got, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	want := filepath.Join(base, "hipoteca-cli", "config.toml")
	if got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}

// TestDataPathRespectsXDGDataHome checks that an absolute XDG_DATA_HOME wins
// wherever it is set: our own rule, on every platform, not just Unix.
func TestDataPathRespectsXDGDataHome(t *testing.T) {
	base := tempDirFor(t)
	t.Setenv("XDG_DATA_HOME", base)

	got, err := DataPath()
	if err != nil {
		t.Fatalf("DataPath() error = %v", err)
	}
	want := filepath.Join(base, "hipoteca-cli", "hipoteca.db")
	if got != want {
		t.Fatalf("DataPath() = %q, want %q", got, want)
	}
}

// TestDataPathFallback checks the platform fallback when XDG_DATA_HOME is
// unset: %LocalAppData% on Windows, ~/.local/share on Linux and the rest of
// Unix, ~/Library/Application Support on macOS.
func TestDataPathFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	switch runtime.GOOS {
	case "windows":
		base := tempDirFor(t)
		t.Setenv("LocalAppData", base)
		got, err := DataPath()
		if err != nil {
			t.Fatalf("DataPath() error = %v", err)
		}
		want := filepath.Join(base, "hipoteca-cli", "hipoteca.db")
		if got != want {
			t.Fatalf("DataPath() = %q, want %q", got, want)
		}
	case "darwin":
		home := tempDirFor(t)
		t.Setenv("HOME", home)
		got, err := DataPath()
		if err != nil {
			t.Fatalf("DataPath() error = %v", err)
		}
		want := filepath.Join(home, "Library", "Application Support", "hipoteca-cli", "hipoteca.db")
		if got != want {
			t.Fatalf("DataPath() = %q, want %q", got, want)
		}
	default:
		home := tempDirFor(t)
		t.Setenv("HOME", home)
		got, err := DataPath()
		if err != nil {
			t.Fatalf("DataPath() error = %v", err)
		}
		want := filepath.Join(home, ".local", "share", "hipoteca-cli", "hipoteca.db")
		if got != want {
			t.Fatalf("DataPath() = %q, want %q", got, want)
		}
	}
}

// TestCacheDirRespectsXDGCacheHome checks that an absolute XDG_CACHE_HOME wins
// on EVERY non-Windows OS, including macOS where os.UserCacheDir alone would
// ignore the variable (our precedence is explicit, see resolveBase).
func TestCacheDirRespectsXDGCacheHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG_CACHE_HOME does not apply on Windows; see TestCacheDirWindows")
	}
	base := tempDirFor(t)
	t.Setenv("XDG_CACHE_HOME", base)
	t.Setenv("HOME", tempDirFor(t))

	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error = %v", err)
	}
	want := filepath.Join(base, "hipoteca-cli")
	if got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
}

// TestCacheDirFallback checks the platform-specific default when
// XDG_CACHE_HOME is empty: ~/.cache on Linux and the rest of Unix,
// ~/Library/Caches on macOS (runtime.GOOS-guarded: the expected value is a
// per-OS fact). On Windows the equivalent is %LocalAppData%, asserted in
// TestCacheDirWindows.
func TestCacheDirFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME-based fallback does not apply on Windows; see TestCacheDirWindows")
	}
	home := tempDirFor(t)
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", home)

	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error = %v", err)
	}
	var wantBase string
	switch runtime.GOOS {
	case "darwin":
		wantBase = filepath.Join(home, "Library", "Caches")
	default:
		wantBase = filepath.Join(home, ".cache")
	}
	want := filepath.Join(wantBase, "hipoteca-cli")
	if got != want {
		t.Fatalf("CacheDir() on %s = %q, want %q", runtime.GOOS, got, want)
	}
}

// TestCacheDirWindows checks the %LocalAppData% resolution on Windows.
func TestCacheDirWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only meaningful on Windows")
	}
	base := tempDirFor(t)
	t.Setenv("LocalAppData", base)

	got, err := CacheDir()
	if err != nil {
		t.Fatalf("CacheDir() error = %v", err)
	}
	want := filepath.Join(base, "hipoteca-cli")
	if got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
}

// TestRelativeXDGRejected: a RELATIVE XDG_* value is an error, never silently
// joined into a CWD-relative path. Config and cache: Unix only (Windows does
// not read those variables). Data: every OS, because XDG_DATA_HOME is ours.
func TestRelativeXDGRejected(t *testing.T) {
	if runtime.GOOS != "windows" {
		home := tempDirFor(t)
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "relativo/config")
		t.Setenv("XDG_CACHE_HOME", "relativo/cache")
		for name, fn := range map[string]func() (string, error){
			"ConfigPath": ConfigPath,
			"CacheDir":   CacheDir,
		} {
			got, err := fn()
			if err == nil {
				t.Fatalf("%s() = %q con XDG relativo; want error", name, got)
			}
		}
	}
	t.Setenv("XDG_DATA_HOME", "relativo/datos")
	got, err := DataPath()
	if err == nil {
		t.Fatalf("DataPath() = %q con XDG_DATA_HOME relativo; want error", got)
	}
}

// TestPathsErrorNeverCWD checks that an unresolvable base directory yields an
// error, never a silent fallback to the current directory. HOME can be unset
// (empty) on Unix: os.UserConfigDir then fails instead of returning ".".
func TestPathsErrorNeverCWD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("AppData/LocalAppData are practically always set on Windows; the empty-HOME failure is Unix-specific")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	for name, fn := range map[string]func() (string, error){
		"ConfigPath": ConfigPath,
		"DataPath":   DataPath,
		"CacheDir":   CacheDir,
	} {
		got, err := fn()
		if err == nil {
			t.Fatalf("%s() = %q with no error; want error, never a fallback to the current directory", name, got)
		}
		if got != "" {
			t.Fatalf("%s() returned path %q alongside an error; want empty path", name, got)
		}
	}
}
