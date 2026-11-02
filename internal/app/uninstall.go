package app

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// ----------------------------------------------------------------------------
// `hipoteca uninstall` (REF-03, S1-S4): el borrado de los datos de la
// herramienta. Guarda tres cosas: la config (con los préstamos), la base de
// datos de las vigilancias y la caché de series. El binario NO está aquí:
// lo ha instalado un gestor de paquetes (Scoop, Homebrew) o el usuario, y
// borrarlo es cosa suya (S4). Las rutas llegan resueltas desde el CLI
// (config.ConfigPath, config.DataPath y config.CacheDir, sustituibles en los
// tests), de modo que este fichero no consulta el entorno.
// ----------------------------------------------------------------------------

// Las tres clases de objetivo del borrado. Son la clave `kind` de la salida.
const (
	UninstallTargetConfig = "config"
	UninstallTargetData   = "data"
	UninstallTargetCache  = "cache"
)

// UninstallTarget es un camino del borrado con su estado: existe, cuánto
// ocupa y si esta ejecución lo ha eliminado.
type UninstallTarget struct {
	// Kind es config, data o cache.
	Kind string `json:"kind"`
	// Path es la ruta exacta del fichero o directorio.
	Path string `json:"path"`
	// Exists dice si existía al lanzar el comando.
	Exists bool `json:"exists"`
	// SizeBytes es el tamaño del fichero, o la suma de su contenido si es
	// un directorio; 0 si no existe.
	SizeBytes int64 `json:"size_bytes"`
	// Removed dice si esta ejecución lo ha borrado (siempre false en un
	// dry-run y en los objetivos que no existían).
	Removed bool `json:"removed"`
}

// UninstallResult es la salida de `uninstall`: los tres objetivos y si la
// ejecución era un dry-run.
type UninstallResult struct {
	SchemaVersion int               `json:"schema_version"`
	DryRun        bool              `json:"dry_run"`
	Targets       []UninstallTarget `json:"targets"`
}

// Uninstall borra la config, la base de datos y la caché de las rutas dadas.
//
//   - sin confirmar (yes false) y sin dry-run no toca nada y responde
//     confirmation_required/2 (S2): borrar la config es borrar los préstamos
//     del usuario, y no hay terminal que interrogue en el contrato del CLI
//     (un valor JSON en stdout), así que la confirmación es la bandera --yes;
//   - con dryRun lista las rutas y sus tamaños sin borrar nada (S3): el
//     dry-run no necesita --yes porque por definición no borra;
//   - con yes borra cada objetivo que exista y devuelve el detalle exacto de
//     lo eliminado (S1). Un objetivo que no existe no es un error.
//
// El binario nunca aparece: el paquete que lo instaló es quien lo retira (S4).
func Uninstall(configPath, dataPath, cacheDir string, yes, dryRun bool) (UninstallResult, error) {
	paths := map[string]string{
		UninstallTargetConfig: configPath,
		UninstallTargetData:   dataPath,
		UninstallTargetCache:  cacheDir,
	}
	targets := make([]UninstallTarget, 0, len(paths))
	for _, kind := range []string{UninstallTargetConfig, UninstallTargetData, UninstallTargetCache} {
		path := paths[kind]
		exists, size, err := pathState(path)
		if err != nil {
			return UninstallResult{}, err
		}
		targets = append(targets, UninstallTarget{
			Kind:      kind,
			Path:      path,
			Exists:    exists,
			SizeBytes: size,
		})
	}
	if !dryRun && !yes {
		return UninstallResult{}, errs.New(errs.KindConfirmationRequired,
			"borrar la configuración (con tus préstamos), la base de datos y la caché es irreversible: no se ha borrado nada",
			"repite el comando con --yes para confirmar el borrado, o con --dry-run para ver qué se borraría y cuánto ocupa")
	}
	if dryRun {
		return UninstallResult{SchemaVersion: model.SchemaVersion, DryRun: true, Targets: targets}, nil
	}
	for i := range targets {
		target := &targets[i]
		if !target.Exists {
			continue
		}
		if err := removePath(target.Path); err != nil {
			return UninstallResult{}, err
		}
		target.Removed = true
	}
	return UninstallResult{SchemaVersion: model.SchemaVersion, DryRun: false, Targets: targets}, nil
}

// pathState informa de si la ruta existe y cuánto ocupa: el tamaño del
// fichero, o la suma del contenido si es un directorio. Una ruta que no
// existe no es un error: es (0, false).
func pathState(path string) (bool, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, errs.New(errs.KindInternal,
			"no se pudo examinar "+path+": "+err.Error(), "")
	}
	if !info.IsDir() {
		return true, info.Size(), nil
	}
	var total int64
	err = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, entryErr error) error {
		if entryErr != nil {
			return entryErr
		}
		if entry.IsDir() {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		total += fileInfo.Size()
		return nil
	})
	if err != nil {
		return true, 0, errs.New(errs.KindInternal,
			"no se pudo medir el contenido de "+path+": "+err.Error(), "")
	}
	return true, total, nil
}

// removePath borra un fichero o un directorio con todo su contenido (la
// caché). El fallo es un error interno que nombra la ruta: nunca se continúa
// a medias sin decirlo.
func removePath(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return errs.New(errs.KindInternal,
			"no se pudo borrar "+path+": "+err.Error(),
			"cierra cualquier proceso de hipoteca que esté corriendo y vuelve a intentarlo")
	}
	return nil
}
