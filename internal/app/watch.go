package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/aam9063/hipoteca-cli/internal/store"
	"github.com/aam9063/hipoteca-cli/internal/watch"
)

// ----------------------------------------------------------------------------
// `watch` (WATCH-01): vigilancias de publicación de índices sobre la base
// local (store) y entrega a los sinks de la config. La capa CLI sólo parsea
// banderas; aquí vive el cableado: abrir la base, resolver los sinks, calcular
// el impacto estimado y ejecutar el bucle de run.
// ----------------------------------------------------------------------------

// WatchEntry es una vigilancia guardada, lista para el contrato de salida.
type WatchEntry struct {
	ID         int64     `json:"id"`
	Kind       string    `json:"kind"`
	Target     string    `json:"target"`
	Sink       string    `json:"sink"`
	WithImpact bool      `json:"with_impact"`
	CreatedAt  time.Time `json:"created_at"`
}

// WatchView es la salida de `watch add`: la vigilancia tal cual quedó.
type WatchView struct {
	SchemaVersion int        `json:"schema_version"`
	Watch         WatchEntry `json:"watch"`
}

// WatchListResult es la salida de `watch list`.
type WatchListResult struct {
	SchemaVersion int          `json:"schema_version"`
	Watches       []WatchEntry `json:"watches"`
}

// WatchRemovedResult es la salida de `watch remove`.
type WatchRemovedResult struct {
	SchemaVersion int   `json:"schema_version"`
	Watch         int64 `json:"watch"`
	Removed       bool  `json:"removed"`
}

// CheckResult es la salida de `watch check`: los eventos de la pasada (uno por
// mes nuevo entregado o reentregado). Sin dato nuevo, events es [] y el comando
// termina con 0 (decisión 5).
type CheckResult struct {
	SchemaVersion int                  `json:"schema_version"`
	Events        []watch.Notification `json:"events"`
}

// watchEntry convierte una fila de la base al contrato de salida.
func watchEntry(w store.Watch) WatchEntry {
	return WatchEntry{
		ID:         w.ID,
		Kind:       w.Kind,
		Target:     w.Target,
		Sink:       w.Sink,
		WithImpact: w.WithImpact,
		CreatedAt:  w.CreatedAt,
	}
}

// openWatchDB abre la base de estado; el llamador la cierra.
func openWatchDB(path string) (*store.DB, error) {
	return store.Open(path)
}

// WatchAdd guarda una vigilancia nueva (WATCH-01.1). El kind de v1 es
// "index" y el target un id del catálogo (not_found/4 si no existe). El sink
// debe existir en la config ([sinks.<nombre>]) y ser de tipo webhook (v1):
// fallar aquí, al configurar, y no en la primera pasada.
func WatchAdd(dbPath, cfgPath string, catalog *bde.Catalog, kind, target, sink string, withImpact bool, now time.Time) (WatchView, error) {
	if kind != watch.KindIndex {
		return WatchView{}, errs.New(errs.KindUsage,
			fmt.Sprintf("el tipo de vigilancia %q no existe; en esta versión sólo hay %q", kind, watch.KindIndex),
			"ejemplo: \"hipoteca watch add index euribor-1y\"")
	}
	if _, err := catalog.Lookup(target); err != nil {
		return WatchView{}, err
	}
	cfg, err := loadConfigAt(cfgPath)
	if err != nil {
		return WatchView{}, err
	}
	if sink == "" {
		sink = "webhook"
	}
	s, ok := cfg.Sinks[sink]
	if !ok {
		return WatchView{}, errs.New(errs.KindUsage,
			fmt.Sprintf("no hay ningún sink llamado %q en la configuración", sink),
			"decláralo en la config:\n\n[sinks."+sink+"]\ntype = \"webhook\"\nurl = \"https://tu-servidor/hook\"")
	}
	if s.Type != "webhook" {
		return WatchView{}, errs.New(errs.KindUsage,
			fmt.Sprintf("el sink %q es de tipo %q; en esta versión sólo hay \"webhook\"", sink, s.Type),
			"cambia el type del sink en la configuración")
	}
	db, err := openWatchDB(dbPath)
	if err != nil {
		return WatchView{}, err
	}
	defer db.Close()
	w, err := db.AddWatch(kind, target, sink, withImpact, now)
	if err != nil {
		return WatchView{}, err
	}
	return WatchView{SchemaVersion: model.SchemaVersion, Watch: watchEntry(w)}, nil
}

// WatchList lista las vigilancias guardadas, por orden de creación. Sin
// vigilancias devuelve watches: [] (no es error).
func WatchList(dbPath string) (WatchListResult, error) {
	db, err := openWatchDB(dbPath)
	if err != nil {
		return WatchListResult{}, err
	}
	defer db.Close()
	ws, err := db.Watches()
	if err != nil {
		return WatchListResult{}, err
	}
	entries := make([]WatchEntry, 0, len(ws))
	for _, w := range ws {
		entries = append(entries, watchEntry(w))
	}
	return WatchListResult{SchemaVersion: model.SchemaVersion, Watches: entries}, nil
}

// WatchRemove borra la vigilancia y su estado de meses vistos. Un id
// desconocido es not_found/4 (lo pone store.RemoveWatch).
func WatchRemove(dbPath string, id int64) (WatchRemovedResult, error) {
	db, err := openWatchDB(dbPath)
	if err != nil {
		return WatchRemovedResult{}, err
	}
	defer db.Close()
	if err := db.RemoveWatch(id); err != nil {
		return WatchRemovedResult{}, err
	}
	return WatchRemovedResult{SchemaVersion: model.SchemaVersion, Watch: id, Removed: true}, nil
}

// LatestSource es la parte del cliente del Banco que necesitan los comandos
// watch: el último valor publicado. *bde.HTTPClient la satisface por
// construcción; los tests inyectan la suya.
type LatestSource interface {
	Latest(ctx context.Context, ids []string) ([]bde.Observation, error)
}

// WatchCheck hace una pasada de vigilancia (WATCH-01.2): detecta meses nuevos,
// graba cada evento una sola vez y lo entrega al sink configurado. Los sinks
// se resuelven contra la config del momento; el impacto estimado se calcula
// contra los préstamos guardados que usan el índice vigilado.
func WatchCheck(ctx context.Context, dbPath, cfgPath string, source LatestSource, catalog *bde.Catalog, now time.Time) (CheckResult, error) {
	cfg, err := loadConfigAt(cfgPath)
	if err != nil {
		return CheckResult{}, err
	}
	db, err := openWatchDB(dbPath)
	if err != nil {
		return CheckResult{}, err
	}
	defer db.Close()
	events, err := watch.Check(ctx, watch.Deps{
		DB:        db,
		Catalog:   catalog,
		Source:    source,
		SinkFor:   sinkResolver(cfg),
		ImpactFor: impactCalculator(cfg),
		Now:       func() time.Time { return now },
	})
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{SchemaVersion: model.SchemaVersion, Events: events}, nil
}

// sinkResolver devuelve la función que convierte un nombre de sink de la
// config en un Sink. Un nombre sin sección es usage/2 que nombra la sección
// que falta; un tipo que no es webhook es usage/2 (v1).
func sinkResolver(cfg config.Config) func(string) (watch.Sink, error) {
	return func(name string) (watch.Sink, error) {
		s, ok := cfg.Sinks[name]
		if !ok {
			return nil, errs.New(errs.KindUsage,
				fmt.Sprintf("no hay ningún sink llamado %q en la configuración", name),
				"decláralo en la config:\n\n[sinks."+name+"]\ntype = \"webhook\"\nurl = \"https://tu-servidor/hook\"")
		}
		if s.Type != "webhook" {
			return nil, errs.New(errs.KindUsage,
				fmt.Sprintf("el sink %q es de tipo %q; en esta versión sólo hay \"webhook\"", name, s.Type),
				"cambia el type del sink en la configuración")
		}
		if s.URL == "" {
			return nil, errs.New(errs.KindUsage,
				fmt.Sprintf("el sink %q no tiene url", name),
				"fija la URL de destino en la configuración")
		}
		return watch.NewWebhook(s.URL), nil
	}
}

// impactCalculator devuelve la función que estima el cambio de cuota de los
// préstamos que usan el índice vigilado. Sólo se llama para vigilancias
// creadas con --with-impact (decisión 3): sin ella, ningún dato del préstamo
// sale de la máquina.
//
// La estimación recalcula la cuota con el capital y el plazo actuales del
// préstamo y el tipo que saldría del nuevo valor del índice (índice +
// diferencial, con suelo y redondeo de la escritura): es una estimación, no la
// revisión real, y así lo dice el aviso. Un préstamo incompleto no produce
// números inventados (P-06): se salta con un warning.
func impactCalculator(cfg config.Config) watch.ImpactFor {
	return func(target string, value model.Decimal) (watch.Impact, []string, error) {
		impact := watch.Impact{Loans: []watch.LoanImpact{}}
		var warnings []string
		ids := make([]string, 0, len(cfg.Loans))
		for id := range cfg.Loans {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			l := cfg.Loans[id]
			if l.Index != target {
				continue
			}
			if len(l.MissingFields()) > 0 {
				warnings = append(warnings,
					fmt.Sprintf("el préstamo %q está incompleto: no se estima su cambio de cuota (falta un dato de tu escritura, P-06)", id))
				continue
			}
			balance, rate, remaining, err := currentBalance(l)
			if err != nil {
				return watch.Impact{}, nil, err
			}
			current, err := finance.Payment(balance, rate, remaining)
			if err != nil {
				return watch.Impact{}, nil, calcUsageError(err)
			}
			newRate, err := rateOf(l, value.Decimal())
			if err != nil {
				return watch.Impact{}, nil, loanError(id, err)
			}
			updated, err := finance.Payment(balance, newRate.Final, remaining)
			if err != nil {
				return watch.Impact{}, nil, calcUsageError(err)
			}
			impact.Loans = append(impact.Loans, watch.LoanImpact{
				Loan:            id,
				Payment:         model.NewMoney(current),
				NewPayment:      model.NewMoney(updated),
				DifferenceMonth: model.NewMoney(updated.Sub(current)),
			})
		}
		if len(impact.Loans) == 0 {
			warnings = append(warnings,
				fmt.Sprintf("ningún préstamo guardado usa el índice %q: el aviso no lleva cambio de cuota", target))
		}
		return impact, warnings, nil
	}
}

// RunParams son los parámetros de WatchRun. Sleep y Report son puntos de
// inyección: Sleep evita esperas reales en los tests y Report convierte un
// fallo de pasada en diagnóstico visible sin matar el bucle (decisión 5).
type RunParams struct {
	DBPath   string
	CfgPath  string
	Source   LatestSource
	Catalog  *bde.Catalog
	Interval time.Duration
	// Now es el reloj inyectado; por defecto time.Now.
	Now func() time.Time
	// Sleep espera el intervalo; devuelve el error del contexto al cancelar.
	// Por defecto un timer que se rinde con el contexto.
	Sleep func(ctx context.Context, d time.Duration) error
	// Report recibe el error de una pasada fallida. Sin Report, el primer
	// error termina el run.
	Report func(error)
}

// WatchRun ejecuta el bucle de `watch run` hasta que se cancela el contexto:
// una pasada de check, esperar el intervalo, repetir. Los fallos de pasada
// (sink caído, Banco sin responder) no matan el bucle cuando hay Report: el
// evento ya está grabado y la siguiente pasada lo reintenta (decisión 5). Al
// cancelar, termina sin error: la cancelación no es un fallo.
func WatchRun(ctx context.Context, p RunParams) error {
	if p.Now == nil {
		p.Now = time.Now
	}
	if p.Sleep == nil {
		p.Sleep = func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}
	db, err := openWatchDB(p.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	cfg, err := loadConfigAt(p.CfgPath)
	if err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		events, err := watch.Check(ctx, watch.Deps{
			DB:        db,
			Catalog:   p.Catalog,
			Source:    p.Source,
			SinkFor:   sinkResolver(cfg),
			ImpactFor: impactCalculator(cfg),
			Now:       p.Now,
		})
		switch {
		case err != nil && p.Report != nil:
			p.Report(err)
		case err != nil:
			return err
		}
		_ = events // run no escribe en stdout: los avisos llegan por el sink
		if err := p.Sleep(ctx, p.Interval); err != nil {
			return nil // el contexto se canceló: fin limpio, no es un fallo
		}
	}
}
