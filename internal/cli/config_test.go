package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/spf13/cobra"
)

// catalogoDePrueba construye el catálogo embebido sin tocar la red.
func catalogoDePrueba(t *testing.T) *bde.Catalog {
	t.Helper()
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("catálogo embebido no válido: %v", err)
	}
	return cat
}

// runtimeDePrueba inyecta un runtime con catálogo y sin cliente del Banco:
// `config set indices.*` mira el catálogo, nunca llama a la red.
func runtimeDePrueba(t *testing.T) func() (*app.Runtime, error) {
	t.Helper()
	cat := catalogoDePrueba(t)
	return func() (*app.Runtime, error) {
		return &app.Runtime{Catalog: cat}, nil
	}
}

// comandoConfig monta el árbol con las rutas y el runtime de prueba.
func comandoConfig(t *testing.T, cfgPath string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := NewRootCmdWithDeps(BuildInfo{Version: "test", Commit: "abc", Date: "x"}, Deps{
		ConfigPath: func() (string, error) { return cfgPath, nil },
		Runtime:    runtimeDePrueba(t),
	})
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(io.Discard)
	return cmd, &stdout
}

// configTemp escribe una config de prueba con un sink con credenciales y
// devuelve su ruta y el secreto que no debe salir nunca por stdout.
func configTemp(t *testing.T) (string, string) {
	t.Helper()
	cfg := config.Default()
	cfg.Sinks["ntfy"] = config.Sink{Type: "webhook", URL: "https://usuario:secreto123@ntfy.sh/mi-topic"}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	return path, "secreto123"
}

// AGENT-01 S6/S7: `config show` saca la config en un único valor JSON y la
// URL de un sink con credenciales sale redactada.
func TestConfigShowRedactsSecrets(t *testing.T) {
	path, secret := configTemp(t)
	cmd, stdout := comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "show"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config show no debe fallar, got %v", err)
	}
	if strings.Contains(stdout.String(), secret) {
		t.Errorf("config show filtra el secreto del sink: %s", stdout.String())
	}
	var out struct {
		SchemaVersion int `json:"schema_version"`
		Config        struct {
			Indices map[string]string `json:"indices"`
			Loans   map[string]any    `json:"loans"`
			Sinks   map[string]struct {
				Type string `json:"type"`
				URL  string `json:"url"`
			} `json:"sinks"`
		} `json:"config"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout no es JSON: %v\n%s", err, stdout.String())
	}
	sink := out.Config.Sinks["ntfy"]
	if sink.Type != "webhook" || !strings.Contains(sink.URL, "redacted@ntfy.sh/mi-topic") {
		t.Errorf("sink = %+v, want webhook con URL redactada", sink)
	}
}

// AGENT-01 S6: `config get` devuelve el valor de una clave y redacta la URL
// de los sinks; una clave inexistente es not_found/4.
func TestConfigGetReturnsAndRedacts(t *testing.T) {
	path, secret := configTemp(t)

	cmd, stdout := comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "get", "sinks.ntfy.url"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(stdout.String(), secret) {
		t.Errorf("config get filtra el secreto: %s", stdout.String())
	}

	cmd, stdout = comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "get", "sinks.ntfy.type"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("got %v", err)
	}
	var out struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("stdout no es JSON: %v", err)
	}
	if out.Key != "sinks.ntfy.type" || out.Value != "webhook" {
		t.Errorf("got %+v", out)
	}

	cmd, _ = comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "get", "sinks.nadie.url"})
	err := cmd.Execute()
	var e *errs.Error
	if err == nil || !errors.As(err, &e) || e.Kind != errs.KindNotFound {
		t.Errorf("una clave inexistente debe ser not_found, got %v", err)
	}
}

// AGENT-01 S6: `config set sinks.<nombre>.url` declara un sink nuevo, valida
// antes de escribir, y guarda la URL real en el disco (la redacción es sólo
// de presentación).
func TestConfigSetDeclaresANewSink(t *testing.T) {
	path, _ := configTemp(t)
	url := "https://token456@ntfy.sh/otro-topic"

	cmd, stdout := comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "set", "sinks.casa.url", url})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(stdout.String(), "token456") {
		t.Errorf("la respuesta no puede hacer eco del token: %s", stdout.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("la config debe seguir siendo legible: %v", err)
	}
	if cfg.Sinks["casa"].URL != url {
		t.Errorf("la URL guardada es %q", cfg.Sinks["casa"].URL)
	}
	if cfg.Sinks["ntfy"].URL != "https://usuario:secreto123@ntfy.sh/mi-topic" {
		t.Error("el set no puede tocar los demás sinks")
	}
}

// AGENT-01 S6: un valor inválido se rechaza como usage/2 y la config queda
// byte a byte igual.
func TestConfigSetRejectsInvalidValueWithoutWriting(t *testing.T) {
	path, _ := configTemp(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cmd, _ := comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "set", "sinks.ntfy.url", "notaurl"})
	err = cmd.Execute()
	var e *errs.Error
	if err == nil || !errors.As(err, &e) || e.Kind != errs.KindUsage {
		t.Fatalf("una URL inválida debe ser usage, got %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("un set rechazado no puede tocar la config")
	}
}

// AGENT-01 S6: los overrides de índice se validan contra el catálogo.
func TestConfigSetIndicesValidatesAgainstCatalog(t *testing.T) {
	path, _ := configTemp(t)

	cmd, _ := comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "set", "indices.nadie", "CODIGO"})
	err := cmd.Execute()
	var e *errs.Error
	if err == nil || !errors.As(err, &e) || e.Kind != errs.KindNotFound {
		t.Fatalf("un id fuera del catálogo debe ser not_found, got %v", err)
	}

	cmd, _ = comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "set", "indices.euribor-1y", "CODIGO_NUEVO"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("got %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Indices["euribor-1y"] != "CODIGO_NUEVO" {
		t.Errorf("el override no llegó a la config: %+v", cfg.Indices)
	}
}

// AGENT-01 S6: los préstamos tienen su propio camino (`loan add`/`edit`);
// `config set` los rechaza con el hint correcto.
func TestConfigSetRejectsLoanKeysWithHint(t *testing.T) {
	path, _ := configTemp(t)
	cmd, _ := comandoConfig(t, path)
	cmd.SetArgs([]string{"config", "set", "loans.casa.spread", "0.90"})
	err := cmd.Execute()
	var e *errs.Error
	if err == nil || !errors.As(err, &e) || e.Kind != errs.KindUsage {
		t.Fatalf("las claves de préstamo deben rechazarse, got %v", err)
	}
	if !strings.Contains(e.Hint, "loan edit") {
		t.Errorf("el hint debe mandar a loan edit, got %q", e.Hint)
	}
}
