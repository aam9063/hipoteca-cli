package app

import (
	"context"
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// Source es la parte del cliente del Banco de España que consumen los
// comandos `index`: el último valor y el histórico. Es una interfaz, no el
// tipo concreto, para que los tests de la capa de orquestación no toquen la
// red; *bde.HTTPClient la satisface por construcción.
type Source interface {
	Latest(ctx context.Context, ids []string) ([]bde.Observation, error)
	History(ctx context.Context, id string, rango bde.Rango) (bde.Series, error)
}

// El cliente de producción satisface la interfaz: si bde cambia de firma, el
// paquete no compila en lugar de fallar en vuelo.
var _ Source = (*bde.HTTPClient)(nil)

// Runtime reúne lo que un comando necesita para ejecutarse: el catálogo de
// series (estático, con los overrides de configuración aplicados) y la fuente
// de datos del Banco.
type Runtime struct {
	Catalog *bde.Catalog
	Source  Source
}

// NewRuntime construye el runtime de producción: lee la configuración local,
// carga el catálogo con sus overrides y prepara el cliente con la caché de
// config.CacheDir() (decisión 6 de CMD-01: `index` usa la caché). Un fallo al
// resolver el directorio de caché no es un error: la caché es descartable
// (decisión 4 de BDE-01) y sin ella la petición simplemente va a la red.
func NewRuntime() (*Runtime, error) {
	path, err := config.ConfigPath()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	catalog, err := bde.LoadCatalog(cfg.Indices)
	if err != nil {
		return nil, err
	}
	var source Source = bde.NewHTTPClient(catalog)
	if dir, err := config.CacheDir(); err == nil {
		source = bde.NewHTTPClientWithCache(catalog, bde.NewCache(dir))
	}
	return &Runtime{Catalog: catalog, Source: source}, nil
}

// IndexValue es un valor puntual de un índice del catálogo, listo para el
// contrato de salida: el mes en formato YYYY-MM y el valor exacto publicado
// por el Banco (P-07) como model.Decimal.
type IndexValue struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Code  string        `json:"code"`
	Month string        `json:"month"`
	Value model.Decimal `json:"value"`
}

// IndexValues es la salida de `index list` y `index latest`: uno o varios
// valores con sus entradas y la atribución de datos sin procesar (P-05).
type IndexValues struct {
	SchemaVersion int               `json:"schema_version"`
	Indices       []IndexValue      `json:"indices"`
	Attribution   model.Attribution `json:"attribution"`
}

// LatestIndices devuelve el último valor publicado de los ids pedidos, en el
// orden pedido. Un id desconocido es not_found/4 antes de tocar la red.
func LatestIndices(ctx context.Context, source Source, catalog *bde.Catalog, ids []string) (IndexValues, error) {
	if len(ids) == 0 {
		return IndexValues{}, errs.New(errs.KindUsage,
			"no se ha indicado ningún índice",
			"pasa uno o más ids del catálogo; \"hipoteca index list\" los muestra")
	}
	for _, id := range ids {
		if _, err := catalog.Lookup(id); err != nil {
			return IndexValues{}, err
		}
	}
	obs, err := source.Latest(ctx, ids)
	if err != nil {
		return IndexValues{}, err
	}
	byCode := make(map[string]bde.Observation, len(obs))
	for _, o := range obs {
		byCode[o.Code] = o
	}
	values := make([]IndexValue, 0, len(ids))
	var last time.Time
	for _, id := range ids {
		idx, err := catalog.Lookup(id) // ya validado arriba
		if err != nil {
			return IndexValues{}, err
		}
		o, ok := byCode[idx.Code]
		if !ok {
			return IndexValues{}, errs.New(errs.KindAPIChanged,
				fmt.Sprintf("la respuesta no trae la serie %s del índice %s", idx.Code, id), "")
		}
		values = append(values, IndexValue{
			ID:    id,
			Name:  idx.Name,
			Code:  o.Code,
			Month: monthString(o.Month),
			Value: o.Value,
		})
		if o.Month.After(last) {
			last = o.Month
		}
	}
	attr, err := rawAttribution(last)
	if err != nil {
		return IndexValues{}, err
	}
	return IndexValues{
		SchemaVersion: model.SchemaVersion,
		Indices:       values,
		Attribution:   attr,
	}, nil
}

// IndexList devuelve todos los índices del catálogo con su último valor.
func IndexList(ctx context.Context, source Source, catalog *bde.Catalog) (IndexValues, error) {
	return LatestIndices(ctx, source, catalog, catalog.IDs())
}

// IndexRef identifica la serie de un histórico: el id pedido y el nombre del
// catálogo (el que se muestra siempre, docs/series.md §2.1) junto al código y
// la descripción que trajo la respuesta.
type IndexRef struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// IndexObservation es un mes del histórico: YYYY-MM y el valor exacto.
type IndexObservation struct {
	Month string        `json:"month"`
	Value model.Decimal `json:"value"`
}

// IndexHistory es la salida de `index show`: el histórico mensual en orden
// ascendente con sus entradas y la atribución de datos sin procesar (P-05).
type IndexHistoryResult struct {
	SchemaVersion int                `json:"schema_version"`
	Index         IndexRef           `json:"index"`
	Range         string             `json:"range"`
	Observations  []IndexObservation `json:"observations"`
	Attribution   model.Attribution  `json:"attribution"`
}

// IndexHistory devuelve el histórico mensual del id pedido para el rango, que
// llega de la CLI ya traducido (decisión 4 de CMD-01).
func IndexHistory(ctx context.Context, source Source, catalog *bde.Catalog, id string, rango bde.Rango) (IndexHistoryResult, error) {
	idx, err := catalog.Lookup(id)
	if err != nil {
		return IndexHistoryResult{}, err
	}
	s, err := source.History(ctx, id, rango)
	if err != nil {
		return IndexHistoryResult{}, err
	}
	if s.Code != idx.Code {
		return IndexHistoryResult{}, errs.New(errs.KindAPIChanged,
			fmt.Sprintf("se pidió la serie %s y la respuesta trae %s", idx.Code, s.Code), "")
	}
	obs := make([]IndexObservation, len(s.Observations))
	for i, o := range s.Observations {
		obs[i] = IndexObservation{Month: monthString(o.Month), Value: o.Value}
	}
	// data_updated_at es el mes más reciente publicado por la fuente
	// (fechaFin de la serie completa), no el final del rango pedido: la
	// atribución data el original, no el recorte.
	attr, err := rawAttribution(s.End)
	if err != nil {
		return IndexHistoryResult{}, err
	}
	return IndexHistoryResult{
		SchemaVersion: model.SchemaVersion,
		Index: IndexRef{
			ID:          idx.ID,
			Code:        s.Code,
			Name:        idx.Name,
			Description: s.Description,
		},
		Range:        rango.String(),
		Observations: obs,
		Attribution:  attr,
	}, nil
}

// rawAttribution construye la atribución de datos sin procesar (P-05) fechada
// al mes más reciente que la fuente tiene publicado: su día 1, en YYYY-MM-DD.
func rawAttribution(last time.Time) (model.Attribution, error) {
	if last.IsZero() {
		return model.Attribution{}, errs.New(errs.KindAPIChanged,
			"la respuesta no trae ninguna fecha de valor para fechar la atribución", "")
	}
	attr, err := model.RawDataAttribution(last.Format("2006-01-02"))
	if err != nil {
		return model.Attribution{}, errs.New(errs.KindInternal, err.Error(), "")
	}
	return attr, nil
}

// monthString renderiza un mes normalizado en el formato del contrato.
func monthString(m time.Time) string {
	return m.Format("2006-01")
}
