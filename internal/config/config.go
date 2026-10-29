package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
)

// SchemaVersion is the version of the config file format. It is independent of
// the output contract (model.SchemaVersion): a breaking change to the config
// bumps this constant, and Load rejects files written by newer or unknown
// versions instead of guessing.
const SchemaVersion = 1

// Config is the local configuration of the user (arranque §7). It holds the
// series-code overrides and the saved loans (LOAN-01); fields are additive
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
	// Loans guarda los préstamos del usuario por id ([loans.<id>],
	// docs/ARRANQUE.md §4.1). Un préstamo incompleto se guarda igual: es
	// dato del usuario y `loan show` avisa de lo que falta (LOAN-01.1);
	// la validación estricta la aplican los comandos que calculan.
	Loans map[string]loan.Loan `toml:"loans,omitempty"`
	// Sinks son los destinos de avisos por nombre ([sinks.<nombre>],
	// WATCH-01): en v1 el único tipo es "webhook" con su URL. Son datos del
	// usuario como los préstamos: viven en la config, no en la base de
	// estado.
	Sinks map[string]Sink `toml:"sinks,omitempty"`
}

// Sink es un destino de avisos configurado por el usuario. En v1 el único
// tipo soportado es "webhook": un POST JSON a URL con el aviso (P-03: por
// defecto el aviso no lleva ningún dato del préstamo).
type Sink struct {
	// Type es "webhook" en v1; otro valor es rechazado al usar el sink.
	Type string `toml:"type"`
	// URL de destino del POST.
	URL string `toml:"url"`
}

// Default returns the configuration to use when no file exists: the current
// SchemaVersion, no overrides and no loans.
func Default() Config {
	return Config{
		SchemaVersion: SchemaVersion,
		Indices:       map[string]string{},
		Loans:         map[string]loan.Loan{},
		Sinks:         map[string]Sink{},
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
	if cfg.Loans == nil {
		cfg.Loans = map[string]loan.Loan{}
	}
	if cfg.Sinks == nil {
		cfg.Sinks = map[string]Sink{}
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
