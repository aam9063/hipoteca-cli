package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// SchemaVersion is the version of the config file format. It is independent of
// the output contract (model.SchemaVersion): a breaking change to the config
// bumps this constant, and Load rejects files written by newer or unknown
// versions instead of guessing.
const SchemaVersion = 1

// Config is the local configuration of the user (arranque §7). It holds the
// series-code overrides and reserves room for the sections LOAN-01 (saved
// loans) and WATCH-01 (watch settings) will add later; fields are additive
// within SchemaVersion.
type Config struct {
	// SchemaVersion is the config format version this file was written with.
	SchemaVersion int `toml:"schema_version"`
	// Indices maps a canonical series id to a Banco de España series code,
	// overriding the catalogue compiled into the binary (e.g.
	// "euribor1y": "SECUENCIA_..."). The overrides do not break the
	// catalogue: validation against internal/bde arrives with BDE-01; until
	// then the map is stored verbatim. An absent or empty map means no
	// override.
	Indices map[string]string `toml:"indices"`
}

// Default returns the configuration to use when no file exists: the current
// SchemaVersion and no overrides.
func Default() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		Indices:       map[string]string{},
	}
}

// Load reads the config at path. A missing file is not an error: it returns
// Default() so a fresh install works without ceremony. A corrupt or unreadable
// file is an explicit error carrying the path, and the file is never
// overwritten: losing the user's data to an automatic "repair" is worse than
// failing (decision 5).
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, errs.New(errs.KindInternal,
			fmt.Sprintf("no se pudo leer la configuración %s: %v", path, err),
			"comprueba los permisos del archivo")
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errs.New(errs.KindInternal,
			fmt.Sprintf("la configuración %s no es un TOML válido: %v", path, err),
			"corrige o elimina el archivo a mano; no se sobrescribe automáticamente")
	}
	if cfg.SchemaVersion != SchemaVersion {
		return Config{}, errs.New(errs.KindInternal,
			fmt.Sprintf("la configuración %s declara schema_version %d; esta versión del programa soporta la %d",
				path, cfg.SchemaVersion, SchemaVersion),
			"actualiza el programa o revisa el archivo a mano; no se sobrescribe automáticamente")
	}
	if cfg.Indices == nil {
		cfg.Indices = map[string]string{}
	}
	return cfg, nil
}

// Save writes the config at path atomically (see writeFileAtomic) with 0600,
// creating the parent directory with 0700 if needed. It refuses to write a
// config with a SchemaVersion it would not read back.
func (c Config) Save(path string) error {
	if c.SchemaVersion != SchemaVersion {
		return errs.New(errs.KindInternal,
			fmt.Sprintf("no se puede guardar la configuración con schema_version %d; esta versión soporta la %d",
				c.SchemaVersion, SchemaVersion),
			"usa config.Default() o el SchemaVersion del paquete")
	}
	data, err := marshal(c)
	if err != nil {
		return errs.New(errs.KindInternal,
			fmt.Sprintf("no se pudo serializar la configuración %s: %v", path, err), "")
	}
	return writeFileAtomic(path, data, 0o600)
}

// marshal serialises a Config to TOML. The library is github.com/pelletier/go-toml/v2
// (see doc.go for the justification).
func marshal(c Config) ([]byte, error) {
	return toml.Marshal(c)
}
