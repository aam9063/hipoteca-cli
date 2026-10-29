package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// webhookTimeout es el tiempo máximo de la entrega al sink: ni un sink caído
// puede colgar un check indefinidamente.
const webhookTimeout = 15 * time.Second

// Webhook es el único sink de v1 (decisión del alcance de WATCH-01): un POST
// JSON con el aviso a la URL configurada por el usuario en su config TOML.
type Webhook struct {
	// URL de destino, tal cual la escribió el usuario.
	URL string
	// Client es el cliente HTTP; NewWebhook pone uno con timeout de 15 s.
	Client *http.Client
}

// NewWebhook construye el sink con su timeout por defecto.
func NewWebhook(url string) *Webhook {
	return &Webhook{URL: url, Client: &http.Client{Timeout: webhookTimeout}}
}

// Deliver envía el payload como POST JSON. Cualquier fallo —la URL no
// responde, el sink contesta un status que no es de éxito— sale como
// upstream_unavailable (código 5, decisión 5): el aviso no se entregó y eso
// es lo único que importa; la pasada siguiente lo reintenta con el evento ya
// grabado.
func (h *Webhook) Deliver(ctx context.Context, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return errs.New(errs.KindInternal,
			"no se pudo serializar el aviso para el sink: "+err.Error(), "")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("la URL del sink webhook %q no es válida: %v", h.URL, err),
			"revisa [sinks] en la configuración")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		return errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("no se pudo contactar con el sink webhook %s: %v", h.URL, err),
			"comprueba la URL del sink en la configuración; el aviso queda grabado y se reintenta")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("el sink webhook respondió HTTP %d; el aviso no se entregó", resp.StatusCode),
			"el aviso queda grabado y la siguiente pasada lo reintenta")
	}
	return nil
}
