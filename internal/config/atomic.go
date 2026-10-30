package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// writeFileAtomic writes data to path so the destination is never left half
// written (decision 4): it creates a temp file in the same directory (same
// filesystem, or Rename would not be atomic), writes, Syncs, closes and
// renames over the destination. The temp file is removed on any failure. The
// parent directory is created with 0700 if missing. Rename replaces an
// existing destination on all supported platforms (Go's os.Rename uses
// MOVEFILE_REPLACE_EXISTING on Windows).
//
// Two known limits, accepted for now:
//   - A hard crash (power loss) between the temp file's creation and the
//     Rename can leave a stale ".<base>.tmp" file behind; later saves do
//     not reclaim it (each save creates a fresh temp file).
//   - Rename replaces a symlink AT path itself, not the file it points to.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("no se pudo crear el directorio %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp")
	if err != nil {
		return fmt.Errorf("no se pudo crear el fichero temporal en %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	// Every exit below removes the temp file; only a successful Rename leaves
	// it behind (renamed into place, not left over).
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("no se pudo fijar el modo %o en %s: %w", perm, tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("no se pudo escribir en %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("no se pudo sincronizar %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("no se pudo cerrar %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("no se pudo mover %s sobre %s: %w", tmpName, path, err)
	}
	tmpName = "" // renamed into place: nothing left to remove

	// Fsync the parent directory so the rename itself survives a power loss:
	// without it, the filesystem can commit the file content but not the
	// rename, and a crash right after "success" reverts the file to its old
	// version. Skipped on Windows, whose API offers no directory handle to
	// sync (os.Open on a directory fails there).
	if runtime.GOOS != "windows" {
		d, err := os.Open(dir)
		if err != nil {
			return fmt.Errorf("no se pudo abrir el directorio %s para sincronizarlo: %w", dir, err)
		}
		if err := d.Sync(); err != nil {
			_ = d.Close()
			return fmt.Errorf("no se pudo sincronizar el directorio %s: %w", dir, err)
		}
		if err := d.Close(); err != nil {
			return fmt.Errorf("no se pudo cerrar el directorio %s: %w", dir, err)
		}
	}
	return nil
}
