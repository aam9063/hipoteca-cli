package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// configConSink escribe una config temporal con un sink que lleva
// credenciales en la URL, como las de un webhook con token.
func configConSink(t *testing.T) (string, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Sinks["ntfy"] = config.Sink{Type: "webhook", URL: "https://usuario:secreto123@ntfy.sh/mi-topic"}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("no se pudo escribir la config de prueba: %v", err)
	}
	return path, "secreto123"
}

// AGENT-01 S7: ni `config show` ni `config get` muestran nunca un secreto:
// la URL del sink con credenciales sale redactada.
func TestConfigShowRedactsSinkCredentials(t *testing.T) {
	path, secret := configConSink(t)
	res, err := ConfigShow(path)
	if err != nil {
		t.Fatalf("ConfigShow no debe fallar, got %v", err)
	}
	wire, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), secret) {
		t.Errorf("config show filtra el secreto del sink: %s", wire)
	}
	sink := res.Config.Sinks["ntfy"]
	if !strings.Contains(sink.URL, "redacted") {
		t.Errorf("la URL del sink debe salir redactada, got %q", sink.URL)
	}
	if !strings.Contains(sink.URL, "ntfy.sh/mi-topic") {
		t.Errorf("la redacción debe conservar host y ruta para seguir siendo útil, got %q", sink.URL)
	}
	if sink.Type != "webhook" {
		t.Errorf("type = %q, want webhook", sink.Type)
	}
}

// AGENT-01 S7: `config get` de la URL del sink también la redacta.
func TestConfigGetRedactsSinkURL(t *testing.T) {
	path, secret := configConSink(t)
	res, err := ConfigGet(path, "sinks.ntfy.url")
	if err != nil {
		t.Fatalf("ConfigGet no debe fallar, got %v", err)
	}
	if strings.Contains(res.Value, secret) {
		t.Errorf("config get filtra el secreto: %q", res.Value)
	}
	if !strings.Contains(res.Value, "redacted") {
		t.Errorf("el valor debe salir redactado, got %q", res.Value)
	}
	if res.Key != "sinks.ntfy.url" {
		t.Errorf("key = %q, want sinks.ntfy.url", res.Key)
	}
}

// AGENT-01 S6: `config get` de un valor sin credenciales lo devuelve tal
// cual; una clave inexistente es not_found/4.
func TestConfigGetPlainValues(t *testing.T) {
	path, _ := configConSink(t)
	res, err := ConfigGet(path, "sinks.ntfy.type")
	if err != nil {
		t.Fatalf("ConfigGet no debe fallar, got %v", err)
	}
	if res.Value != "webhook" {
		t.Errorf("value = %q, want webhook", res.Value)
	}
	_, err = ConfigGet(path, "sinks.nadie.url")
	if !isErrsKind(err, errs.KindNotFound) {
		t.Errorf("una clave inexistente debe ser not_found, got %v", err)
	}
	_, err = ConfigGet(path, "indices.nadie")
	if !isErrsKind(err, errs.KindNotFound) {
		t.Errorf("un override inexistente debe ser not_found, got %v", err)
	}
}

// AGENT-01 S6: `config set` valida ANTES de escribir: una URL inválida es
// usage/2 y la config queda byte a byte igual.
func TestConfigSetRejectsInvalidSinkURLBeforeWriting(t *testing.T) {
	path, _ := configConSink(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"notaurl", "ftp://ntfy.sh/x", "https:///sin-host"} {
		_, err := ConfigSet(path, nil, "sinks.ntfy.url", url)
		if !isErrsKind(err, errs.KindUsage) {
			t.Errorf("la URL %q debe ser rechazada como usage, got %v", url, err)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("un set rechazado no puede tocar la config")
	}
}

// AGENT-01 S6: `config set` de un tipo distinto de webhook se rechaza.
func TestConfigSetRejectsUnknownSinkType(t *testing.T) {
	path, _ := configConSink(t)
	if _, err := ConfigSet(path, nil, "sinks.ntfy.type", "email"); !isErrsKind(err, errs.KindUsage) {
		t.Errorf("el tipo email debe rechazarse como usage, got %v", err)
	}
}

// AGENT-01 S6: `config set sinks.<nombre>.url` declara un sink nuevo (el
// hueco de WATCH-01) y lo escribe con la disciplina atómica 0600.
func TestConfigSetWritesNewSinkAtomically(t *testing.T) {
	path, _ := configConSink(t)
	url := "https://token456@ntfy.sh/otro-topic"
	res, err := ConfigSet(path, nil, "sinks.casa.url", url)
	if err != nil {
		t.Fatalf("ConfigSet no debe fallar, got %v", err)
	}
	if strings.Contains(res.Value, "token456") {
		t.Errorf("la respuesta no puede hacer eco del secreto: %q", res.Value)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("la config escrita debe poder leerse: %v", err)
	}
	if cfg.Sinks["casa"].URL != url {
		t.Errorf("la URL guardada es %q, want %q (la redacción es sólo de presentación)", cfg.Sinks["casa"].URL, url)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("permiso = %o, want 600 (disciplina de internal/config)", info.Mode().Perm())
		}
	}
}

// AGENT-01 S6: un override de índice sólo vale para un id del catálogo.
func TestConfigSetIndicesRequiresCatalogID(t *testing.T) {
	path, _ := configConSink(t)
	if _, err := ConfigSet(path, nil, "indices.nadie", "CODIGO"); !isErrsKind(err, errs.KindNotFound) {
		t.Errorf("un id fuera del catálogo debe ser not_found, got %v", err)
	}
	catalog, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConfigSet(path, catalog, "indices.euribor-1y", "CODIGO_NUEVO")
	if err != nil {
		t.Fatalf("ConfigSet de un id del catálogo no debe fallar, got %v", err)
	}
	if res.Value != "CODIGO_NUEVO" {
		t.Errorf("value = %q, want CODIGO_NUEVO", res.Value)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Indices["euribor-1y"] != "CODIGO_NUEVO" {
		t.Errorf("el override no llegó a la config: %+v", cfg.Indices)
	}
}

// AGENT-01 S6: los préstamos no se tocan desde `config set`: su camino es
// `loan add`/`loan edit`.
func TestConfigSetRejectsLoanKeys(t *testing.T) {
	path, _ := configConSink(t)
	if _, err := ConfigSet(path, nil, "loans.casa.spread", "0.90"); !isErrsKind(err, errs.KindUsage) {
		t.Errorf("las claves de préstamo deben rechazarse con el hint de loan edit, got %v", err)
	}
}

// AGENT-01 S7: la redacción es la misma política que los errores del
// webhook (internal/watch/webhook.go): userinfo censurado, host y ruta
// intactos, y una URL que no se puede parsear jamás se hace eco.
func TestRedactedSinkURLPolicy(t *testing.T) {
	// La redacción conserva el esquema, host y ruta, y censura el userinfo
	// completo, igual que internal/watch/webhook.go.
	cases := map[string]string{
		"https://usuario:secreto123@ntfy.sh/mi-topic": "https://redacted@ntfy.sh/mi-topic",
		"https://ntfy.sh/mi-topic":                    "https://ntfy.sh/mi-topic",
	}
	for in, want := range cases {
		if got := redactedSinkURL(in); got != want {
			t.Errorf("redactedSinkURL(%q) = %q, want %q", in, got, want)
		}
	}
	if got := redactedSinkURL("://no es una url"); strings.Contains(got, "no es una url") {
		t.Errorf("una URL que no parsea no se hace eco, got %q", got)
	}
}

// isErrsKind informa de si err lleva el kind del contrato indicado, incluso
// envuelto.
func isErrsKind(err error, kind errs.Kind) bool {
	var e *errs.Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Kind == kind
}
