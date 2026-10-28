package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// appName is the exact directory name under every base directory (arranque
// §7): hipoteca-cli/config.toml, hipoteca-cli/hipoteca.db and hipoteca-cli/
// for the cache.
const appName = "hipoteca-cli"

// ConfigPath returns the path of the user's config file:
// <base>/hipoteca-cli/config.toml. The base follows an explicit precedence,
// identical on Linux, macOS and every other Unix:
//
//  1. XDG_CONFIG_HOME when it is set and absolute;
//  2. otherwise the platform's own answer: os.UserConfigDir — $HOME/.config
//     on Linux and the rest of Unix, ~/Library/Application Support on macOS,
//     %AppData% on Windows.
//
// The variable is checked here instead of relying on os.UserConfigDir
// because the stdlib only honours XDG on Linux: on macOS it would ignore
// XDG_CONFIG_HOME and answer with ~/Library/Application Support. On Windows
// XDG_CONFIG_HOME does not apply at all. An empty or relative base is an
// error: the path is never relative, because writing the user's mortgage
// data next to the binary (the current directory) would be a silent
// surprise.
func ConfigPath() (string, error) {
	xdg := "XDG_CONFIG_HOME"
	if runtime.GOOS == "windows" {
		xdg = "" // Windows never reads XDG_CONFIG_HOME: %AppData% is the platform answer.
	}
	base, err := resolveBase(xdg, os.UserConfigDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName, "config.toml"), nil
}

// DataPath returns the path of the local database (watch data, events):
// <base>/hipoteca-cli/hipoteca.db. The base follows the same explicit
// precedence, on EVERY platform (the stdlib has no UserDataDir):
//
//  1. XDG_DATA_HOME when it is set and absolute;
//  2. otherwise the platform fallback: %LocalAppData% on Windows,
//     $HOME/.local/share on Linux and the rest of Unix,
//     ~/Library/Application Support on macOS.
//
// An empty or relative base is an error: the path is never relative.
func DataPath() (string, error) {
	base, err := resolveBase("XDG_DATA_HOME", dataFallback)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName, "hipoteca.db"), nil
}

// CacheDir returns the directory of the series cache:
// <base>/hipoteca-cli/. The base follows the same explicit precedence as
// ConfigPath:
//
//  1. XDG_CACHE_HOME when it is set and absolute (Unix only);
//  2. otherwise the platform's own answer: os.UserCacheDir — $HOME/.cache on
//     Linux and the rest of Unix, ~/Library/Caches on macOS, %LocalAppData%
//     on Windows.
//
// An empty or relative base is an error: the path is never relative.
func CacheDir() (string, error) {
	xdg := "XDG_CACHE_HOME"
	if runtime.GOOS == "windows" {
		xdg = "" // Windows never reads XDG_CACHE_HOME: %LocalAppData% is the platform answer.
	}
	base, err := resolveBase(xdg, os.UserCacheDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

// resolveBase implements the shared precedence for the three base
// directories:
//
//  1. when xdgVar is set to an absolute path, that value;
//  2. otherwise the platform's own answer, returned by fallback;
//  3. when neither yields a non-empty absolute path, an error — never a
//     relative one, and never a silent fallback to the current directory.
//
// An empty xdgVar (XDG does not apply: config and cache on Windows) skips
// step 1.
func resolveBase(xdgVar string, fallback func() (string, error)) (string, error) {
	if xdgVar != "" {
		if xdg := os.Getenv(xdgVar); xdg != "" {
			if !filepath.IsAbs(xdg) {
				return "", fmt.Errorf("la variable %s debe ser una ruta absoluta; es %q", xdgVar, xdg)
			}
			return xdg, nil
		}
	}
	base, err := fallback()
	if err != nil {
		return "", err
	}
	if base == "" || !filepath.IsAbs(base) {
		return "", fmt.Errorf("no se pudo resolver un directorio base absoluto; el valor es %q", base)
	}
	return base, nil
}

// dataFallback answers with the platform's data directory when XDG_DATA_HOME
// is not set: %LocalAppData% on Windows, $HOME/.local/share on Linux and the
// rest of Unix, ~/Library/Application Support on macOS (the same directory
// os.UserConfigDir answers with for config there).
func dataFallback() (string, error) {
	if runtime.GOOS == "windows" {
		return os.Getenv("LocalAppData"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support"), nil
	}
	return filepath.Join(home, ".local", "share"), nil
}
