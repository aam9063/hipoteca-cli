package bde

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

const (
	// defaultTimeout es el timeout de la petición completa (decisión 6).
	defaultTimeout = 15 * time.Second
	// defaultUserAgent identifica al cliente ante el Banco (docs/series.md §7).
	// La versión concreta la enchufará CMD-01; mientras tanto, "dev".
	defaultUserAgent = "hipoteca-cli/dev"
	// defaultRetryWait es la espera del único reintento permitido.
	defaultRetryWait = 500 * time.Millisecond
	// maxBodyBytes acota el cuerpo aceptado; el fixture mayor (MAX) son ~16 KB.
	maxBodyBytes = 10 << 20
	// languageES es el idioma obligatorio de la API (docs/series.md §2.1): la
	// caché y los textos mostrados asumen "es", y siempre se envía.
	languageES = "es"
)

const (
	endpointFavoritas   = "favoritas"
	endpointListaSeries = "listaSeries"
)

// HTTPClient es el cliente de la API de estadísticas del Banco de España
// (docs/series.md). Es el único sitio del programa con permiso para hacer
// peticiones HTTP.
//
// Invariantes que el tipo cumple por construcción: toda petición lleva
// `idioma=es` y `series`; nunca se envía un rango que la API rechace (decisión
// 2); y ningún número pasa por float64 — el cuerpo se decodifica con
// json.Number + UseNumber y se convierte a model.Decimal (decisión 1).
type HTTPClient struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	retryWait  time.Duration
	catalog    *Catalog
	cache      *Cache
}

// NewHTTPClient construye el cliente contra la base del catálogo embebido, con
// timeout de 15 s, User-Agent hipoteca-cli/dev y un único reintento con backoff
// ante 5xx/429 (decisión 6). El catálogo debe ser no nulo: es quien traduce
// id → código y clasifica los errores por-elemento.
func NewHTTPClient(catalog *Catalog) *HTTPClient {
	if catalog == nil {
		panic("bde: NewHTTPClient necesita un catálogo no nulo")
	}
	return &HTTPClient{
		httpClient: &http.Client{Timeout: defaultTimeout},
		baseURL:    catalog.EndpointBase(),
		userAgent:  defaultUserAgent,
		retryWait:  defaultRetryWait,
		catalog:    catalog,
	}
}

// NewHTTPClientWithCache construye el cliente con caché local bajo el
// directorio dado (config.CacheDir()); ver cache.go para la política.
func NewHTTPClientWithCache(catalog *Catalog, cache *Cache) *HTTPClient {
	c := NewHTTPClient(catalog)
	c.cache = cache
	return c
}

// Latest devuelve el último valor publicado de cada id del catálogo, mediante
// `favoritas?idioma=es&series=...`. El array de respuesta sigue el orden de los
// códigos pedidos (docs/series.md §3.1) y las observaciones llegan en ese orden.
//
// Si algún elemento del array trae errNum, se separan: las observaciones
// correctas se devuelven junto a un único error del contrato que agrega las
// series fallidas (docs/series.md §6). Con caché, un acierto no toca la red.
func (c *HTTPClient) Latest(ids []string) ([]Observation, error) {
	if len(ids) == 0 {
		return nil, errs.New(errs.KindUsage,
			"no se ha indicado ningún índice",
			"pasa al menos un id del catálogo de series")
	}
	codes := make([]string, 0, len(ids))
	for _, id := range ids {
		idx, err := c.catalog.Lookup(id)
		if err != nil {
			return nil, err
		}
		codes = append(codes, idx.Code)
	}
	return c.LatestCodes(codes)
}

// LatestCodes es Latest para códigos de serie directos: lo usan las series
// auxiliares del IRPH (docs/series.md §1.4) y los códigos que el usuario fuerza
// en la configuración. Un errNum sobre un código que sale del catálogo es
// api_changed/7; sobre uno que no, not_found/4 (decisión 5).
func (c *HTTPClient) LatestCodes(codes []string) ([]Observation, error) {
	if len(codes) == 0 {
		return nil, errs.New(errs.KindUsage,
			"no se ha indicado ninguna serie",
			"pasa al menos un código de serie")
	}
	key := strings.Join(codes, ",")
	if c.cache != nil {
		if obs, ok := c.cache.GetLatest(key); ok {
			return obs, nil
		}
	}
	body, err := c.get(endpointFavoritas, url.Values{"series": {key}})
	if err != nil {
		return nil, err
	}
	obs, err := c.decodeFavoritas(body)
	if err != nil {
		// Las observaciones correctas del array pueden acompañar al error;
		// la entrada fallida no se cachea.
		return obs, err
	}
	if c.cache != nil {
		c.cache.PutLatest(key, obs)
	}
	return obs, nil
}

// History devuelve el histórico mensual de un id en orden ascendente, mediante
// `listaSeries?idioma=es&series=...&rango=...` (docs/series.md §2, §3.2).
func (c *HTTPClient) History(id string, rango Rango) (Series, error) {
	idx, err := c.catalog.Lookup(id)
	if err != nil {
		return Series{}, err
	}
	if rango.value == "" {
		return Series{}, errs.New(errs.KindUsage,
			"no se ha indicado el rango del histórico",
			"usa Rango30M, Rango60M, RangoMAX o RangoAnio")
	}
	key := idx.Code + "|" + rango.value
	if c.cache != nil {
		if s, ok := c.cache.GetHistory(key, rango); ok {
			return s, nil
		}
	}
	params := url.Values{"series": {idx.Code}, "rango": {rango.value}}
	body, err := c.get(endpointListaSeries, params)
	if err != nil {
		return Series{}, err
	}
	s, err := c.decodeHistoria(body)
	if err != nil {
		return Series{}, err
	}
	if c.cache != nil {
		c.cache.PutHistory(key, rango, s)
	}
	return s, nil
}

// get hace la petición GET y devuelve el cuerpo. El error, si lo hay, ya lleva
// el sobre del contrato (docs/series.md §6, arranque §6.4). `idioma=es` se
// añade aquí: ninguna llamada puede olvidarlo (invariante por construcción).
func (c *HTTPClient) get(endpoint string, params url.Values) ([]byte, error) {
	params.Set("idioma", languageES)
	u := strings.TrimRight(c.baseURL, "/") + "/" + endpoint + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.New(errs.KindInternal, "petición inválida al construirse: "+err.Error(), "")
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errs.New(errs.KindRateLimited,
			"el Banco de España está limitando las peticiones (HTTP 429)",
			"inténtalo de nuevo dentro de unos minutos")
	case resp.StatusCode >= 500:
		return nil, errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("la API del Banco de España no está disponible (HTTP %d)", resp.StatusCode),
			"inténtalo de nuevo más tarde")
	case resp.StatusCode != http.StatusOK:
		// El único status de éxito del contrato es 200; los errores de
		// validación llegan con 200 y cuerpo propio (docs/series.md §6).
		return nil, apiChangedf("la API respondió HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, transportError(err)
	}
	return body, nil
}

// doWithRetry ejecuta la petición con como máximo un reintento ante 5xx/429
// (decisión 6): sin reintentos infinitos ni bucles. Los cortes de conexión y
// los timeouts no se reintentan: suben tal cual a transportError.
func (c *HTTPClient) doWithRetry(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
		return resp, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	time.Sleep(c.retryWait)
	return c.httpClient.Do(req)
}

// transportError clasifica un fallo del transporte: timeout del cliente →
// upstream_unavailable/5; cualquier otro corte → network/6 (docs/series.md §6).
func transportError(err error) error {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return errs.New(errs.KindUpstreamUnavailable,
			"la API del Banco de España no respondió a tiempo: "+err.Error(),
			"inténtalo de nuevo más tarde")
	}
	return errs.New(errs.KindNetwork,
		"no se pudo contactar con la API del Banco de España: "+err.Error(),
		"comprueba la conexión a internet")
}

// apiChangedf construye el error de contrato roto: código 7 con enlace a issue
// implícito en el hint (docs/series.md §6).
func apiChangedf(format string, args ...any) *errs.Error {
	return errs.New(errs.KindAPIChanged,
		"la API del Banco de España cambió: "+fmt.Sprintf(format, args...),
		"el contrato está en docs/series.md; abre una issue para renovar el cliente")
}

// ---------------------------------------------------------------- decodificación

// jsonKind mira el primer byte no blanco: '[' (array de éxito), '{' (objeto de
// error) o 0 (no es JSON). La asimetría array-vs-objeto de docs/series.md §6 se
// resuelve aquí, antes de decodificar nada.
func jsonKind(body []byte) byte {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		case '[', '{':
			return b
		default:
			return 0
		}
	}
	return 0
}

// decodeJSON decodifica el cuerpo con UseNumber: los números JSON llegan como
// json.Number con su texto original y se convierten a model.Decimal con
// NewFromString, que respeta la cadena exacta del JSON (decisión 1, P-07).
func decodeJSON(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(v)
}

// apiErrorBody es la forma del objeto de error del Banco (HTTP 200): errNum,
// errMsgUsr y errMsgDebug (docs/series.md §6).
type apiErrorBody struct {
	ErrNum      int    `json:"errNum"`
	ErrMsgUsr   string `json:"errMsgUsr"`
	ErrMsgDebug string `json:"errMsgDebug"`
}

// decodeAPIError decodifica un cuerpo que debía ser el objeto de error.
func decodeAPIError(body []byte) (apiErrorBody, error) {
	var e apiErrorBody
	if err := decodeJSON(body, &e); err != nil {
		return e, apiChangedf("el cuerpo de error no es el objeto esperado: %v", err)
	}
	return e, nil
}

// classifyAPIError aplica la tabla de docs/series.md §6 para el objeto de
// error de listaSeries/favoritas:
//
//	412 + "no existe" en errMsgDebug → not_found/4  (error-listaSeries-serie-inexistente.json)
//	cualquier otro errNum o forma    → api_changed/7 (error-listaSeries-rango-invalido.json,
//	                                                  error-listaSeries-sin-idioma.json,
//	                                                  error-favoritas-sin-series.json)
func (c *HTTPClient) classifyAPIError(e apiErrorBody) *errs.Error {
	if e.ErrNum == 412 && strings.Contains(e.ErrMsgDebug, "no existe") {
		return errs.New(errs.KindNotFound,
			"el Banco de España no conoce la serie pedida: "+e.ErrMsgDebug, "")
	}
	return apiChangedf("la API devolvió errNum %d (%s)",
		e.ErrNum, firstNonEmpty(e.ErrMsgDebug, e.ErrMsgUsr))
}

// rawFavorita es un elemento del array de favoritas, de éxito o de error:
// los elementos de error traen `codigo` y `errNum` y nada más
// (docs/series.md §3.1, §6). Valor llega como json.Number y nunca como float.
type rawFavorita struct {
	Serie            string      `json:"serie"`
	Codigo           string      `json:"codigo"`
	ErrNum           *int        `json:"errNum"`
	DescripcionCorta string      `json:"descripcionCorta"`
	CodFrecuencia    string      `json:"codFrecuencia"`
	Decimales        int         `json:"decimales"`
	Simbolo          string      `json:"simbolo"`
	Tendencia        string      `json:"tendencia"`
	FechaValor       string      `json:"fechaValor"`
	Valor            json.Number `json:"valor"`
}

// decodeFavoritas decodifica la respuesta de favoritas: array de elementos o
// objeto de error. Devuelve las observaciones correctas aunque haya elementos
// con errNum; en ese caso el error agrega las series fallidas.
func (c *HTTPClient) decodeFavoritas(body []byte) ([]Observation, error) {
	switch jsonKind(body) {
	case '{':
		apiErr, err := decodeAPIError(body)
		if err != nil {
			return nil, err
		}
		return nil, c.classifyAPIError(apiErr)
	case '[':
		var elems []rawFavorita
		if err := decodeJSON(body, &elems); err != nil {
			return nil, apiChangedf("favoritas no es un array válido: %v", err)
		}
		return c.favoritasObservations(elems)
	default:
		return nil, apiChangedf("la respuesta no es JSON")
	}
}

// favoritasObservations valida el array por elemento (docs/series.md §6): los
// elementos con errNum se separan y se clasifican; los correctos se convierten.
func (c *HTTPClient) favoritasObservations(elems []rawFavorita) ([]Observation, error) {
	obs := make([]Observation, 0, len(elems))
	type failure struct {
		code string
		err  *errs.Error
	}
	var failures []failure
	for _, e := range elems {
		switch {
		case e.ErrNum != nil:
			failures = append(failures, failure{e.Codigo, c.classifyElementError(e.Codigo, *e.ErrNum)})
		default:
			o, err := observationFromFavorita(e)
			if err != nil {
				failures = append(failures, failure{e.Serie, err})
				continue
			}
			obs = append(obs, o)
		}
	}
	if len(failures) > 0 {
		codes := make([]string, 0, len(failures))
		changed := false
		for _, f := range failures {
			codes = append(codes, f.code)
			if f.err.Kind == errs.KindAPIChanged {
				changed = true
			}
		}
		if changed {
			// Un fallo sobre un código del catálogo es un cambio de contrato
			// y manda sobre los not_found (decisión 5).
			return obs, errs.New(errs.KindAPIChanged,
				fmt.Sprintf("el Banco de España devolvió error para las series del catálogo: %s", strings.Join(codes, ", ")),
				"el contrato está en docs/series.md; abre una issue para renovar el cliente")
		}
		return obs, errs.New(errs.KindNotFound,
			fmt.Sprintf("las series %s no existen en el Banco de España", strings.Join(codes, ", ")),
			"revisa los códigos; si vienen de la configuración, corrígelos")
	}
	return obs, nil
}

// classifyElementError clasifica un elemento de favoritas con errNum
// (docs/series.md §6): catálogo → api_changed/7; código forzado → not_found/4.
func (c *HTTPClient) classifyElementError(code string, errNum int) *errs.Error {
	if c.catalog.hasCode(code) {
		return apiChangedf("la serie del catálogo %s devolvió errNum %d", code, errNum)
	}
	return errs.New(errs.KindNotFound,
		fmt.Sprintf("la serie %s no existe en el Banco de España (errNum %d)", code, errNum),
		"revisa el código; si viene de la configuración, corrígelo")
}

// observationFromFavorita convierte un elemento de éxito: serie + fechaValor +
// valor exacto. La hora de fechaValor no es significativa (docs/series.md §5).
func observationFromFavorita(e rawFavorita) (Observation, *errs.Error) {
	if e.Serie == "" {
		return Observation{}, apiChangedf("un elemento de favoritas no identifica su serie")
	}
	v, err := model.NewDecimal(e.Valor.String())
	if err != nil {
		return Observation{}, apiChangedf("la serie %s trae un valor que no es un decimal válido: %q", e.Serie, e.Valor.String())
	}
	month, err := monthOf(e.FechaValor)
	if err != nil {
		return Observation{}, apiChangedf("la serie %s trae una fecha inesperada: %q", e.Serie, e.FechaValor)
	}
	return Observation{Code: e.Serie, Month: month, Value: v}, nil
}

// rawListaSerie es el objeto de una serie en la respuesta de listaSeries
// (docs/series.md §3.2). fechas y valores van en el mismo orden y descendente;
// los números llegan como json.Number.
type rawListaSerie struct {
	Serie            string        `json:"serie"`
	Descripcion      string        `json:"descripcion"`
	DescripcionCorta string        `json:"descripcionCorta"`
	CodFrecuencia    string        `json:"codFrecuencia"`
	Decimales        int           `json:"decimales"`
	Simbolo          string        `json:"simbolo"`
	FechaInicio      string        `json:"fechaInicio"`
	FechaFin         string        `json:"fechaFin"`
	Fechas           []string      `json:"fechas"`
	Valores          []json.Number `json:"valores"`
}

// decodeHistoria decodifica la respuesta de listaSeries: array con un objeto
// por serie, u objeto de error.
func (c *HTTPClient) decodeHistoria(body []byte) (Series, error) {
	switch jsonKind(body) {
	case '{':
		apiErr, err := decodeAPIError(body)
		if err != nil {
			return Series{}, err
		}
		return Series{}, c.classifyAPIError(apiErr)
	case '[':
		var elems []rawListaSerie
		if err := decodeJSON(body, &elems); err != nil {
			return Series{}, apiChangedf("listaSeries no es un array válido: %v", err)
		}
		if len(elems) != 1 {
			return Series{}, apiChangedf("listaSeries devolvió %d series; se pedía una", len(elems))
		}
		return c.seriesFromLista(elems[0])
	default:
		return Series{}, apiChangedf("la respuesta no es JSON")
	}
}

// seriesFromLista convierte el objeto de la serie: invierte el orden
// descendente de la API una vez, aquí, para que Series sea siempre ascendente.
func (c *HTTPClient) seriesFromLista(e rawListaSerie) (Series, error) {
	if e.Serie == "" {
		return Series{}, apiChangedf("listaSeries no identifica la serie devuelta")
	}
	if len(e.Fechas) == 0 || len(e.Fechas) != len(e.Valores) {
		return Series{}, apiChangedf("la serie %s trae %d fechas y %d valores", e.Serie, len(e.Fechas), len(e.Valores))
	}
	start, err := monthOf(e.FechaInicio)
	if err != nil {
		return Series{}, apiChangedf("la serie %s trae una fechaInicio inesperada: %q", e.Serie, e.FechaInicio)
	}
	end, err := monthOf(e.FechaFin)
	if err != nil {
		return Series{}, apiChangedf("la serie %s trae una fechaFin inesperada: %q", e.Serie, e.FechaFin)
	}
	obs := make([]Observation, 0, len(e.Fechas))
	for i := len(e.Fechas) - 1; i >= 0; i-- {
		month, err := monthOf(e.Fechas[i])
		if err != nil {
			return Series{}, apiChangedf("la serie %s trae una fecha inesperada: %q", e.Serie, e.Fechas[i])
		}
		v, err := model.NewDecimal(e.Valores[i].String())
		if err != nil {
			return Series{}, apiChangedf("la serie %s trae un valor que no es un decimal válido: %q", e.Serie, e.Valores[i].String())
		}
		obs = append(obs, Observation{Code: e.Serie, Month: month, Value: v})
	}
	return Series{
		Code:             e.Serie,
		Description:      e.Descripcion,
		DescriptionShort: e.DescripcionCorta,
		Frequency:        e.CodFrecuencia,
		Decimals:         e.Decimales,
		Unit:             e.Simbolo, // opcional en la API: puede llegar vacío
		Start:            start,
		End:              end,
		Observations:     obs,
	}, nil
}

// monthOf convierte un instante ISO 8601 de la API al mes que representa:
// día 1, 00:00 UTC (docs/series.md §5: la hora no es significativa ni estable).
func monthOf(instant string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, instant)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
