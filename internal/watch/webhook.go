package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	// Client es el cliente HTTP; NewWebhook pone uno con timeout de 15 s y
	// sin seguir redirecciones (P-03).
	Client *http.Client
}

// NewWebhook construye el sink con su timeout por defecto. El cliente NO
// sigue redirecciones: una 3xx de un sink no puede usarse para que Go
// reenvíe el aviso —con los datos del préstamo dentro, si se creó con
// --with-impact— a otro host (P-03: el único destino del aviso es el sink
// que el usuario configuró). La 3xx queda como respuesta final y, al no ser
// 2xx, es un fallo del sink con el evento a salvo para el reintento.
func NewWebhook(url string) *Webhook {
	return &Webhook{
		URL: url,
		Client: &http.Client{
			Timeout: webhookTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Deliver envía el payload como POST JSON. Cualquier fallo —la URL no
// responde, el sink contesta un status que no es de éxito, el contexto se
// cancela— sale con su kind del contrato: un fallo del sink es
// upstream_unavailable (código 5, decisión 5) y una cancelación es
// cancelled (código 2). En ambos casos el evento ya grabado se reintenta en
// la pasada siguiente.
func (h *Webhook) Deliver(ctx context.Context, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return errs.New(errs.KindInternal,
			"no se pudo serializar el aviso para el sink: "+err.Error(), "")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		return errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("la URL del sink webhook %q no es válida: %v", redactedURL(h.URL), err),
			"revisa [sinks] en la configuración")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.Client.Do(req)
	if err != nil {
		// Una cancelación (Ctrl+C vía el contexto del llamador) no es un
		// fallo del sink: es el usuario interrumpiendo la entrega.
		if ctx.Err() != nil {
			return errs.New(errs.KindCancelled,
				"la entrega del aviso al sink webhook se ha cancelado: "+ctx.Err().Error(),
				"el aviso queda grabado y la siguiente pasada lo reintenta")
		}
		return errs.New(errs.KindUpstreamUnavailable,
			fmt.Sprintf("no se pudo contactar con el sink webhook %s: %v", redactedURL(h.URL), transportCause(err)),
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

// transportCause extrae la causa de un fallo de transporte quitando la
// envoltura de net/http (*url.Error), que repite la URL —con el usuario en
// claro aunque la contraseña va censurada— en su texto. El detalle útil es la
// causa (timeout, DNS, conexión rechazada), no la URL.
func transportCause(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// redactedURL prepara una URL para un mensaje de error: censura el userinfo
// completo (una URL de sink puede llevar usuario y contraseña o token,
// p. ej. https://user:token@host) y conserva el host y la ruta para que el
// mensaje siga siendo útil. Una URL que no se puede parsear no se hace eco:
// es texto arbitrario del usuario.
func redactedURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "(URL del sink no válida)"
	}
	if u.User != nil {
		u.User = url.User("redacted")
	}
	return u.String()
}
