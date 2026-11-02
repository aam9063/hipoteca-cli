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
	// la validación estricta la aplican los comandos que calculan. El
	// esquema del préstamo es de este paquete (REF-02); sus reglas de
	// validez, de internal/loan.
	Loans map[string]Loan `toml:"loans,omitempty"`
	// Sinks son los destinos de avisos por nombre ([sinks.<nombre>],
	// WATCH-01): en v1 el único tipo es "webhook" con su URL. Son datos del
	// usuario como los préstamos: viven en la config, no en la base de
	// estado.
	Sinks map[string]Sink `toml:"sinks,omitempty"`
}

// Sync es la resincronización con el recibo real del banco (opcional;
// docs/ARRANQUE.md §4.1), tal como vive en la config ([loans.<id>.sync]).
// Cuando está presente, el capital pendiente y el tipo de partida son los del
// recibo, no los calculados (decisión 2 de LOAN-01), así que sus tres campos
// son obligatorios juntos. El esquema del fichero es de config (REF-02:
// config no importa a loan); las reglas de validez las aplica internal/loan.
type Sync struct {
	// Date es la fecha del recibo, YYYY-MM-DD.
	Date string `toml:"date" json:"date"`
	// OutstandingPrincipal es el capital pendiente según el recibo, en euros,
	// como decimal exacto ("152341.18").
	OutstandingPrincipal string `toml:"outstanding_principal" json:"outstanding_principal"`
	// CurrentRate es el TIN vigente según el recibo, en tanto por ciento
	// ("3.12").
	CurrentRate string `toml:"current_rate" json:"current_rate"`
}

// Loan es un préstamo guardado en la config local ([loans.<id>],
// docs/ARRANQUE.md §4.1). Aquí está su forma en el fichero — el esquema que
// config guarda, carga y valida como suyo (REF-02): los importes y los tipos
// son cadenas decimales exactas — el formato TOML del arranque conserva la
// escala que escribió el usuario ("180000.00" sigue siendo "180000.00")— y
// las reglas de validez (decimales bien formados, rangos, presencia de los
// campos de la escritura) viven en internal/loan, que no es lo mismo que el
// cargador de ficheros. internal/loan expone estos tipos vía alias, así que
// el resto del programa sigue hablando de loan.Loan.
//
// Los campos que dependen de la escritura son obligatorios o explícitos
// (decisión 1): ReferenceOffsetMonths y FloorRate van con puntero porque el
// cero y la cadena vacía son valores legítimos (offset 0 y "sin suelo"), de
// modo que su ausencia en el TOML (puntero nil) es lo que marca el préstamo
// como incompleto. FloorRate vacío no es un suelo inventado: es la ausencia
// de suelo declarada.
type Loan struct {
	// Name es el nombre con el que el usuario reconoce el préstamo.
	Name string `toml:"name" json:"name"`
	// Type es variable, mixta o fija (§4.1).
	Type string `toml:"type" json:"type"`
	// StartDate es la fecha de la primera cuota o de firma, según escritura,
	// YYYY-MM-DD.
	StartDate string `toml:"start_date" json:"start_date"`
	// OriginalPrincipal es el capital original en euros ("180000.00").
	OriginalPrincipal string `toml:"original_principal" json:"original_principal"`
	// TermMonths es el plazo total en meses.
	TermMonths int `toml:"term_months" json:"term_months"`
	// PaymentDay es el día del mes en que se paga la cuota (1..31).
	PaymentDay int `toml:"payment_day" json:"payment_day"`
	// InitialRate es el TIN anual % del periodo inicial ("1.25").
	InitialRate string `toml:"initial_rate" json:"initial_rate"`
	// InitialPeriodMonths son los meses a tipo fijo antes de la primera
	// revisión; 0 es un valor legítimo y por eso va con puntero.
	InitialPeriodMonths *int `toml:"initial_period_months" json:"initial_period_months"`
	// Index es el id del índice del catálogo ("euribor-1y").
	Index string `toml:"index" json:"index"`
	// Spread es el diferencial en puntos ("0.89").
	Spread string `toml:"spread" json:"spread"`
	// SpreadUnbonified es el diferencial sin bonificaciones; opcional.
	SpreadUnbonified string `toml:"spread_unbonified,omitempty" json:"spread_unbonified,omitempty"`
	// RevisionEveryMonths es la frecuencia de revisión en meses.
	RevisionEveryMonths int `toml:"revision_every_months" json:"revision_every_months"`
	// ReferenceOffsetMonths es qué mes de índice se usa: N meses antes del mes
	// de revisión, según escritura; 0 es legítimo y por eso va con puntero.
	ReferenceOffsetMonths *int `toml:"reference_offset_months" json:"reference_offset_months"`
	// FloorRate es el suelo del tipo ("2.00") o "" para sin suelo; su ausencia
	// (nil) deja el préstamo incompleto.
	FloorRate *string `toml:"floor_rate" json:"floor_rate"`
	// RateRounding es el redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05.
	RateRounding string `toml:"rate_rounding" json:"rate_rounding"`
	// InterestConvention es la convención de interés; hoy sólo
	// monthly-nominal (TIN/12 por mes).
	InterestConvention string `toml:"interest_convention" json:"interest_convention"`
	// PaymentRounding es el redondeo de la cuota; hoy sólo half-up-cents.
	PaymentRounding string `toml:"payment_rounding" json:"payment_rounding"`
	// EarlyRepaymentFeePct es la comisión de amortización anticipada en
	// tanto por ciento del importe según escritura ("0" si no la hay).
	EarlyRepaymentFeePct string `toml:"early_repayment_fee_pct" json:"early_repayment_fee_pct"`
	// Sync es la última sincronización con el recibo; opcional.
	Sync *Sync `toml:"sync,omitempty" json:"sync,omitempty"`
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
		Loans:         map[string]Loan{},
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
			"comprueba los permisos del fichero")
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, errs.New(errs.KindInternal,
			fmt.Sprintf("la configuración %s no es un TOML válido: %v", path, err),
			"corrige o elimina el fichero a mano; no se sobrescribe automáticamente")
	}
	if cfg.SchemaVersion != SchemaVersion {
		return Config{}, errs.New(errs.KindInternal,
			fmt.Sprintf("la configuración %s declara schema_version %d; esta versión del programa soporta la %d",
				path, cfg.SchemaVersion, SchemaVersion),
			"actualiza el programa o revisa el fichero a mano; no se sobrescribe automáticamente")
	}
	if cfg.Indices == nil {
		cfg.Indices = map[string]string{}
	}
	if cfg.Loans == nil {
		cfg.Loans = map[string]Loan{}
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
