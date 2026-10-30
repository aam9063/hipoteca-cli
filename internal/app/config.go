package app

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// Las operaciones `config show|get|set` cierran el hueco declarado en
// WATCH-01: hasta hoy declarar un sink obligaba a editar el TOML a mano.
// `set` valida antes de escribir y guarda con la disciplina atómica y 0600
// de internal/config (Config.Save). S7: ningún secreto sale por stdout —
// una URL de sink con credenciales se muestra redactada con la misma
// política que los errores del webhook (internal/watch/webhook.go), y la
// URL real sólo vive en el disco.

// SinkView es un sink tal como sale por stdout: la URL redactada.
type SinkView struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// ConfigView es la forma wire de la config: la misma estructura que
// internal/config escribe, con las URLs de los sinks redactadas (S7).
type ConfigView struct {
	SchemaVersion int                  `json:"schema_version"`
	Indices       map[string]string    `json:"indices"`
	Loans         map[string]loan.Loan `json:"loans"`
	Sinks         map[string]SinkView  `json:"sinks"`
}

// ConfigShowResult es la salida de `config show`.
type ConfigShowResult struct {
	SchemaVersion int        `json:"schema_version"`
	Path          string     `json:"path"`
	Config        ConfigView `json:"config"`
}

// ConfigValueResult es la salida de `config get` y `config set`: la clave
// pedida y su valor (redactado si es una URL de sink).
type ConfigValueResult struct {
	SchemaVersion int    `json:"schema_version"`
	Key           string `json:"key"`
	Value         string `json:"value"`
}

// ConfigShow devuelve la config tal cual está en el disco, con las URLs de
// los sinks redactadas (S7). Los préstamos se muestran como están: son datos
// locales que el usuario ya ve con `loan show` y aquí no salen de la máquina.
func ConfigShow(path string) (ConfigShowResult, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return ConfigShowResult{}, err
	}
	return ConfigShowResult{
		SchemaVersion: model.SchemaVersion,
		Path:          path,
		Config:        viewDe(cfg),
	}, nil
}

// ConfigGet devuelve el valor de una clave puntual. Las claves son rutas
// dotted sobre la config:
//
//	indices.<id>           el código de serie sobrescrito del índice
//	sinks.<nombre>.type    el tipo del sink
//	sinks.<nombre>.url     la URL del sink (redactada en la salida, S7)
//
// Una clave que no existe es not_found/4.
func ConfigGet(path, key string) (ConfigValueResult, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return ConfigValueResult{}, err
	}
	value, err := lookup(cfg, key)
	if err != nil {
		return ConfigValueResult{}, err
	}
	return ConfigValueResult{
		SchemaVersion: model.SchemaVersion,
		Key:           key,
		Value:         presentable(key, value),
	}, nil
}

// ConfigSet valida la clave y el valor ANTES de tocar nada y, si son
// correctos, escribe la config con Config.Save (atómica, 0600). La respuesta
// hace eco de la clave y del valor presentable — redactado si es una URL de
// sink (S7) —; el valor real sólo queda en el disco.
//
// Las claves admitidas son las mismas de `config get`. Los préstamos NO se
// tocan desde aquí: su camino es `loan add`/`loan edit` (el hint lo dice).
func ConfigSet(path string, catalog *bde.Catalog, key, value string) (ConfigValueResult, error) {
	if value == "" {
		return ConfigValueResult{}, errs.New(errs.KindUsage,
			"falta el valor: indícalo tras la clave",
			"ejemplo: hipoteca config set sinks.ntfy.url https://ntfy.sh/mi-topic")
	}
	if err := validate(key, value, catalog); err != nil {
		return ConfigValueResult{}, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return ConfigValueResult{}, err
	}
	if err := assign(&cfg, key, value); err != nil {
		return ConfigValueResult{}, err
	}
	if err := cfg.Save(path); err != nil {
		return ConfigValueResult{}, err
	}
	return ConfigValueResult{
		SchemaVersion: model.SchemaVersion,
		Key:           key,
		Value:         presentable(key, value),
	}, nil
}

// parseKey trocea la clave dotted y la clasifica. Devuelve el error de uso
// con las formas válidas cuando no encaja en ninguna.
func parseKey(key string) (kind, first, second string, err error) {
	parts := strings.Split(key, ".")
	switch {
	case len(parts) == 2 && parts[0] == "indices":
		if parts[1] == "" {
			break
		}
		return "indices", parts[1], "", nil
	case len(parts) == 3 && parts[0] == "sinks":
		if parts[1] == "" || (parts[2] != "type" && parts[2] != "url") {
			return "", "", "", errs.New(errs.KindUsage,
				fmt.Sprintf("la clave %q no es correcta: un sink se dirige como sinks.<nombre>.type o sinks.<nombre>.url", key),
				"ejemplo: hipoteca config get sinks.ntfy.url")
		}
		return "sinks", parts[1], parts[2], nil
	case len(parts) >= 1 && parts[0] == "loans":
		return "", "", "", errs.New(errs.KindUsage,
			fmt.Sprintf("los préstamos no se editan desde config: usa \"hipoteca loan edit\" para %q", key),
			"\"hipoteca loan edit <id> --<campo> <valor>\" cambia un campo; \"hipoteca loan list\" muestra los ids guardados")
	}
	return "", "", "", errs.New(errs.KindUsage,
		fmt.Sprintf("la clave %q no es correcta", key),
		"las claves son indices.<id>, sinks.<nombre>.type y sinks.<nombre>.url; \"hipoteca config show\" muestra todo")
}

// lookup resuelve la clave sobre la config cargada. Una clave con forma
// válida pero valor inexistente es not_found/4.
func lookup(cfg config.Config, key string) (string, error) {
	kind, first, second, err := parseKey(key)
	if err != nil {
		return "", err
	}
	switch kind {
	case "indices":
		code, ok := cfg.Indices[first]
		if !ok {
			return "", errs.New(errs.KindNotFound,
				fmt.Sprintf("no hay ningún override de código para el índice %q", first),
				"\"hipoteca config show\" muestra la config; el código por defecto está en el catálogo")
		}
		return code, nil
	case "sinks":
		sink, ok := cfg.Sinks[first]
		if !ok {
			return "", errs.New(errs.KindNotFound,
				fmt.Sprintf("no existe ningún sink llamado %q", first),
				"\"hipoteca config show\" lista los sinks; declara uno nuevo con \"hipoteca config set sinks.<nombre>.url <URL>\"")
		}
		if second == "type" {
			return sink.Type, nil
		}
		return sink.URL, nil
	}
	return "", errs.New(errs.KindUsage, fmt.Sprintf("la clave %q no es correcta", key), "")
}

// validate comprueba el valor ANTES de abrir la config: un valor que no
// podría usarse nunca no llega a tocar el disco.
func validate(key, value string, catalog *bde.Catalog) error {
	kind, first, _, err := parseKey(key)
	if err != nil {
		return err
	}
	switch kind {
	case "indices":
		if catalog == nil {
			// Sin catálogo no se puede verificar el id: no se escribe un
			// override que el catálogo podría no reconocer.
			return errs.New(errs.KindNotFound,
				fmt.Sprintf("no hay catálogo con el que validar el índice %q", first),
				"el id debe existir en \"hipoteca index list\"")
		}
		if _, err := catalog.Lookup(first); err != nil {
			return err
		}
	case "sinks":
		if strings.HasSuffix(key, ".type") && value != "webhook" {
			return errs.New(errs.KindUsage,
				fmt.Sprintf("el tipo de sink %q no está soportado: en v1 el único tipo es \"webhook\"", value),
				"un webhook es un POST JSON a la URL que configures")
		}
		if strings.HasSuffix(key, ".url") {
			return validateSinkURL(value)
		}
	}
	return nil
}

// validateSinkURL es la validación de una URL de sink: debe ser una URL
// http(s) con host. La misma URL se valida al entregar el aviso; rechazarla
// aquí evita guardar una errata que sólo fallaría en el primer aviso.
func validateSinkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return errs.New(errs.KindUsage,
			fmt.Sprintf("la URL del sink %q no es válida", redactedSinkURL(raw)),
			"usa una URL http(s) completa con host, p. ej. https://ntfy.sh/mi-topic")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errs.New(errs.KindUsage,
			fmt.Sprintf("la URL del sink %s usa el esquema %q: sólo se admiten http y https", redactedSinkURL(raw), u.Scheme),
			"usa una URL http(s) completa con host, p. ej. https://ntfy.sh/mi-topic")
	}
	return nil
}

// assign aplica el cambio sobre la config cargada. La clave ya está
// validada; aquí sólo se escribe el valor en su sitio.
func assign(cfg *config.Config, key, value string) error {
	kind, first, second, err := parseKey(key)
	if err != nil {
		return err
	}
	switch kind {
	case "indices":
		if cfg.Indices == nil {
			cfg.Indices = map[string]string{}
		}
		cfg.Indices[first] = value
	case "sinks":
		if cfg.Sinks == nil {
			cfg.Sinks = map[string]config.Sink{}
		}
		sink := cfg.Sinks[first]
		if second == "type" {
			sink.Type = value
		} else {
			sink.URL = value
		}
		cfg.Sinks[first] = sink
	}
	return nil
}

// presentable prepara el valor para stdout: la URL de un sink se redacta
// (S7); todo lo demás sale tal cual.
func presentable(key, value string) string {
	if strings.HasPrefix(key, "sinks.") && strings.HasSuffix(key, ".url") {
		return redactedSinkURL(value)
	}
	return value
}

// viewDe construye la vista wire de la config: sinks con la URL redactada
// (S7) y los mapas siempre presentes (nunca null en el JSON, §5.2).
func viewDe(cfg config.Config) ConfigView {
	indices := cfg.Indices
	if indices == nil {
		indices = map[string]string{}
	}
	loans := cfg.Loans
	if loans == nil {
		loans = map[string]loan.Loan{}
	}
	sinks := map[string]SinkView{}
	for name, s := range cfg.Sinks {
		sinks[name] = SinkView{Type: s.Type, URL: redactedSinkURL(s.URL)}
	}
	return ConfigView{
		SchemaVersion: cfg.SchemaVersion,
		Indices:       indices,
		Loans:         loans,
		Sinks:         sinks,
	}
}

// redactedSinkURL censura el userinfo completo de una URL de sink (una URL
// puede llevar usuario y contraseña o token, p. ej. https://user:token@host)
// conservando host y ruta para que el valor siga siendo útil. Es la MISMA
// política de los errores del webhook (internal/watch/webhook.go); está
// duplicada aquí porque internal/watch está fuera del alcance de AGENT-01.
// Una URL que no se puede parsear no se hace eco: es texto arbitrario del
// usuario.
func redactedSinkURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(URL del sink no válida)"
	}
	if u.User != nil {
		u.User = url.User("redacted")
	}
	return u.String()
}
