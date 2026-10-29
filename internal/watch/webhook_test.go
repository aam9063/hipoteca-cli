package watch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// ----------------------------------------------------------------------------
// Sink webhook (WATCH-01.5): un POST JSON a la URL configurada con la
// atribución dentro. Los tests usan httptest: ningún test toca la red.
// ----------------------------------------------------------------------------

// TestWebhookEntregaElPayloadConAtribucion: el POST llega con el JSON del
// payload (que incluye la atribución y su fecha) y Content-Type application/json.
func TestWebhookEntregaElPayloadConAtribucion(t *testing.T) {
	var gotBody []byte
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("método = %s, want POST", r.Method)
		}
		gotContentType = r.Header.Get("Content-Type")
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("leer el cuerpo: %v", err)
		}
		gotBody = b
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook := NewWebhook(srv.URL)
	payload := map[string]any{
		"event": "index.published",
		"attribution": map[string]any{
			"text":            "Fuente: Sitio web del Banco de España (www.bde.es)",
			"data_updated_at": "2026-10-01",
			"not_endorsed":    true,
		},
	}
	if err := hook.Deliver(context.Background(), payload); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	var decoded map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("el cuerpo recibido no es JSON válido: %v\n%s", err, gotBody)
	}
	attr, ok := decoded["attribution"].(map[string]any)
	if !ok {
		t.Fatalf("el payload recibido no lleva el bloque attribution:\n%s", gotBody)
	}
	if attr["text"] != "Fuente: Sitio web del Banco de España (www.bde.es)" || attr["data_updated_at"] != "2026-10-01" {
		t.Errorf("la atribución recibida = %+v, want el texto del Banco con su fecha", attr)
	}
}

// TestWebhookHTTP500EsUpstreamUnavailable: un fallo del sink es código 5
// (decisión 5), no un error interno.
func TestWebhookHTTP500EsUpstreamUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	hook := NewWebhook(srv.URL)
	err := hook.Deliver(context.Background(), map[string]any{"event": "index.published"})
	if err == nil {
		t.Fatal("Deliver con HTTP 500: se esperaba error")
	}
	env, ok := errs.From(err)
	if !ok {
		t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
	}
	if env.Error.ExitCode != 5 {
		t.Errorf("código = %d, want 5", env.Error.ExitCode)
	}
}

// TestWebhookInalcanzableEsUpstreamUnavailable: un sink que no responde es
// también un fallo del sink (código 5), no un error de red distinto: el aviso
// no se entregó y eso es lo que importa.
func TestWebhookInalcanzableEsUpstreamUnavailable(t *testing.T) {
	hook := NewWebhook("http://127.0.0.1:1/nunca")
	err := hook.Deliver(context.Background(), map[string]any{"event": "index.published"})
	if err == nil {
		t.Fatal("Deliver a un sink inalcanzable: se esperaba error")
	}
	env, ok := errs.From(err)
	if !ok {
		t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
	}
	if env.Error.ExitCode != 5 {
		t.Errorf("código = %d, want 5", env.Error.ExitCode)
	}
}
