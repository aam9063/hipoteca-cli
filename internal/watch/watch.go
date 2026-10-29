package watch

import (
	"context"
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/aam9063/hipoteca-cli/internal/store"
)

// El evento y el kind únicos de v1 (arranque §5.3, WATCH-01).
const (
	// EventIndexPublished es el evento: se ha publicado un mes nuevo de la
	// serie que sigue la vigilancia.
	EventIndexPublished = "index.published"
	// KindIndex es el único kind de vigilancia en v1.
	KindIndex = "index"
)

// Source es la parte del cliente del Banco de España que necesita el check:
// el último valor publicado de las series. *bde.HTTPClient la satisface.
type Source interface {
	Latest(ctx context.Context, ids []string) ([]bde.Observation, error)
}

// Sink es un destino de avisos. El único sink de v1 es el webhook
// (webhook.go); los tests inyectan los suyos.
type Sink interface {
	Deliver(ctx context.Context, payload any) error
}

// LoanImpact es el cambio estimado de cuota de UN préstamo (sólo viaja en el
// aviso si la vigilancia se creó con --with-impact, decisión 3).
type LoanImpact struct {
	Loan            string        `json:"loan"`
	Payment         model.Decimal `json:"payment"`
	NewPayment      model.Decimal `json:"new_payment"`
	DifferenceMonth model.Decimal `json:"difference_month"`
}

// Impact es el bloque de impacto del aviso, siempre marcado como estimación
// cuando viaja: no es un dato del banco ni del préstamo real, es la cuota
// recalculada con el capital y el plazo actuales.
type Impact struct {
	Estimated bool         `json:"estimated"`
	Note      string       `json:"note"`
	Loans     []LoanImpact `json:"loans"`
}

// Notification es el aviso: el payload que viaja al sink y, a la vez, el
// evento que `watch check` imprime (un único JSON por evento, P-01). Sin
// --with-impact Impact es nil y NO viaja ningún dato del préstamo: ni id, ni
// capital, ni cuota (decisión 3).
type Notification struct {
	SchemaVersion int               `json:"schema_version"`
	Event         string            `json:"event"`
	Watch         NotificationWatch `json:"watch"`
	Index         NotificationIndex `json:"index"`
	Month         string            `json:"month"`
	Value         model.Decimal     `json:"value"`
	Impact        *Impact           `json:"impact,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
	Attribution   model.Attribution `json:"attribution"`
}

// NotificationWatch identifica la vigilancia que originó el aviso.
type NotificationWatch struct {
	Kind       string `json:"kind"`
	Target     string `json:"target"`
	Sink       string `json:"sink"`
	WithImpact bool   `json:"with_impact"`
}

// NotificationIndex identifica la serie: el id del catálogo, su nombre para
// personas y el código del Banco de España.
type NotificationIndex struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Series string `json:"series"`
}

// ImpactFor calcula el impacto estimado del nuevo valor del índice sobre los
// préstamos del usuario que lo usan. Vive en internal/app (necesita config,
// loan y finance); aquí es un punto de inyección para no traerlos a este
// paquete. Warnings son avisos del cálculo (préstamo incompleto, ninguno usa
// el índice) que acompañan al aviso.
type ImpactFor func(target string, value model.Decimal) (Impact, []string, error)

// Deps reúne lo que Check necesita: el estado (store), el catálogo, la fuente
// del Banco, la resolución de sinks y el reloj.
type Deps struct {
	DB      *store.DB
	Catalog *bde.Catalog
	Source  Source
	// SinkFor resuelve el nombre del sink configurado en un Sink. Se llama
	// sólo cuando hay algo que entregar (decisión 5: sin dato nuevo no hay
	// error, ni siquiera si el sink está mal configurado).
	SinkFor func(name string) (Sink, error)
	// ImpactFor sólo se llama para vigilancias creadas con --with-impact.
	ImpactFor ImpactFor
	// Now es el reloj inyectado.
	Now func() time.Time
}

// Check hace una pasada: para cada vigilancia, el último valor publicado del
// índice se compara con los meses ya vistos en la base. Cada mes genuinamente
// nuevo produce exactamente un evento: se graba primero (la clave única hace
// imposible el duplicado, decisión 2) y después se entrega al sink. Un evento
// grabado no entregado (fallo anterior del sink) se reintenta en cada pasada
// antes que los meses nuevos, sin duplicar avisos (decisión 5).
//
// Sin vigilancias o sin dato nuevo devuelve una lista vacía y ningún error.
// Un fallo del sink es error (código 5) pero no borra lo ya grabado.
func Check(ctx context.Context, deps Deps) ([]Notification, error) {
	watches, err := deps.DB.Watches()
	if err != nil {
		return nil, err
	}
	events := []Notification{}
	if len(watches) == 0 {
		return events, nil
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}

	// Una sola petición al Banco para todas las vigilancias: los ids van
	// agrupados y la respuesta se indexa por código de serie.
	ids := make([]string, 0, len(watches))
	codes := make(map[string]bde.Index, len(watches))
	for _, w := range watches {
		if _, seen := codes[w.Target]; seen {
			continue
		}
		idx, err := deps.Catalog.Lookup(w.Target)
		if err != nil {
			return events, err
		}
		codes[w.Target] = idx
		ids = append(ids, w.Target)
	}
	obs, err := deps.Source.Latest(ctx, ids)
	if err != nil {
		return events, err
	}
	byCode := make(map[string]bde.Observation, len(obs))
	for _, o := range obs {
		byCode[o.Code] = o
	}

	now := deps.Now()
	for _, w := range watches {
		idx := codes[w.Target]
		o, ok := byCode[idx.Code]
		if !ok {
			return events, errs.New(errs.KindAPIChanged,
				fmt.Sprintf("la respuesta no trae la serie %s del índice %s", idx.Code, w.Target), "")
		}
		month := o.Month.Format("2006-01")

		seen, err := deps.DB.SeenMonths(w.ID)
		if err != nil {
			return events, err
		}

		// Reintentos primero: eventos ya grabados que el sink no recibió.
		for _, s := range seen {
			if s.DeliveredAt != nil {
				continue
			}
			n, err := deps.buildNotification(w, idx, s.Month, s.Value, now)
			if err != nil {
				return events, err
			}
			if err := deliver(ctx, deps, w.Sink, n); err != nil {
				return events, err
			}
			if err := deps.DB.MarkDelivered(w.ID, s.Month, now); err != nil {
				return events, err
			}
			events = append(events, n)
		}

		// Mes genuinamente nuevo: grabar y entregar, una sola vez.
		hasMonth := false
		for _, s := range seen {
			if s.Month == month {
				hasMonth = true
				break
			}
		}
		if hasMonth {
			continue
		}
		inserted, err := deps.DB.RecordSeen(w.ID, month, o.Value.String(), now)
		if err != nil {
			return events, err
		}
		if !inserted {
			continue // otra pasada lo grabó mientras tanto: no duplicar
		}
		n, err := deps.buildNotification(w, idx, month, o.Value.String(), now)
		if err != nil {
			return events, err
		}
		if err := deliver(ctx, deps, w.Sink, n); err != nil {
			return events, err
		}
		if err := deps.DB.MarkDelivered(w.ID, month, now); err != nil {
			return events, err
		}
		events = append(events, n)
	}
	return events, nil
}

// buildNotification construye el aviso para un mes ya fijado: índice, mes,
// valor exacto, serie y atribución (decisión 4); con --with-impact, el bloque
// de impacto marcado y la advertencia P-03 (decisión 3).
func (d Deps) buildNotification(w store.Watch, idx bde.Index, month, value string, now time.Time) (Notification, error) {
	v, err := model.NewDecimal(value)
	if err != nil {
		return Notification{}, errs.New(errs.KindInternal,
			fmt.Sprintf("el valor guardado del mes %s de %s no es un decimal válido: %q", month, w.Target, value), "")
	}
	attr, err := model.RawDataAttribution(month + "-01")
	if err != nil {
		return Notification{}, errs.New(errs.KindInternal, err.Error(), "")
	}
	n := Notification{
		SchemaVersion: model.SchemaVersion,
		Event:         EventIndexPublished,
		Watch: NotificationWatch{
			Kind:       w.Kind,
			Target:     w.Target,
			Sink:       w.Sink,
			WithImpact: w.WithImpact,
		},
		Index: NotificationIndex{
			ID:     idx.ID,
			Name:   idx.Name,
			Series: idx.Code,
		},
		Month:       month,
		Value:       v,
		Attribution: attr,
	}
	if !w.WithImpact {
		return n, nil
	}
	// Decisión 3: el aviso lleva la advertencia de que un dato del préstamo
	// sale de la máquina hacia el sink (P-03).
	n.Warnings = append(n.Warnings,
		"este aviso incluye un dato de tu préstamo (el cambio estimado de cuota) porque la vigilancia se creó con --with-impact: quien pueda leer el sink puede leerlo (P-03)")
	if d.ImpactFor == nil {
		return Notification{}, errs.New(errs.KindInternal,
			"la vigilancia pide impacto pero nadie calcula el impacto estimado", "")
	}
	impact, warnings, err := d.ImpactFor(w.Target, v)
	if err != nil {
		return Notification{}, err
	}
	// Sin préstamos que impactar no se cuelga un bloque vacío: el aviso
	// explica en warnings por qué no lleva cambio de cuota.
	if len(impact.Loans) > 0 {
		impact.Estimated = true
		if impact.Note == "" {
			impact.Note = "estimación: cuota recalculada con el capital y el plazo actuales del préstamo; no es un dato de tu banco"
		}
		n.Impact = &impact
	}
	n.Warnings = append(n.Warnings, warnings...)
	return n, nil
}

// deliver resuelve el sink nombrado y entrega el aviso con el contexto del
// llamador: una cancelación (Ctrl+C) debe poder abortar una entrega en
// vuelo. El sink se resuelve aquí, en el último momento: un sink mal
// configurado no estorba mientras no haya nada que entregar (decisión 5).
func deliver(ctx context.Context, deps Deps, sinkName string, n Notification) error {
	if deps.SinkFor == nil {
		return errs.New(errs.KindInternal, "no hay forma de resolver el sink del aviso", "")
	}
	sink, err := deps.SinkFor(sinkName)
	if err != nil {
		return err
	}
	return sink.Deliver(ctx, n)
}
