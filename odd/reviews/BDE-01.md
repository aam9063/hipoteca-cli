# Review gate · BDE-01: cliente del Banco de España, catálogo y caché

## Cambio bajo revisión

- **Tarea:** BDE-01 — `odd/tasks/BDE-01-cliente.md`.
- **Rama:** `hc/bde-01-cliente` (base `dev`).
- **Riesgo declarado:** alto, ítem 6 (un fallo de parseo entra antes de cualquier
  cálculo y ningún test de `internal/finance` lo ve).
- **Verificador independiente:** sí, escalado el ítem 6 y aceptado por el plan
  (`independentVerifier: true`).

## Veredicto del verificador: PASS WITH FIXES

Lo que **confirmó sólido**, con 19 sondas adversariales propias:

- **Exactitud decimal**: `3.10`, `3.1e0`, `3.10E1`, `-2.5e-1`, `1e400`, 33 dígitos,
  `4447519` y `-0.272` conservan su escala. Nada pasa por `float64`; el pipeline
  `json.Number` → `model.NewDecimal(texto original)` resiste.
- **Reintentos**: exactamente uno en 5xx/429, ninguno en 4xx ni en éxito; 429→éxito
  recupera con dos hits.
- **Caché**: borde exacto de las 24 h, histórico cerrado sin caducidad, entradas corruptas,
  ajenas o de versión futura ignoradas, fallo de escritura tolerado, cuatro rangos sin
  colisión.
- **Portabilidad**: `monthOf` idéntico bajo tres zonas horarias; sin `syscall` ni supuestos
  de sistema de archivos; nombres de fixture coincidentes byte a byte.

## Los cinco hallazgos y su corrección

| Sev. | Hallazgo | Corrección |
|---|---|---|
| **major** | `favoritas` con `[]` o con menos elementos que códigos pedidos devolvía **éxito con datos faltantes**, y se cacheaba. El contrato exige `api_changed`. | Exactamente un elemento por código, en orden, con la `serie`/`codigo` correctos. Array vacío, corto, duplicado o desordenado → `api_changed`/7 nombrando lo esperado y lo recibido. |
| minor | La inversión del histórico era **ciega**: una respuesta ascendente se devolvía descendente y se cacheaba en silencio. | Se verifica el orden (no creciente) **antes** de invertir; si no, `api_changed`/7. |
| minor | Un objeto de error sobre un código **del catálogo** se clasificaba `not_found`/4. | `api_changed`/7: si el código es nuestro, la forma rota es del contrato, no una serie ausente. `not_found` queda para códigos forzados fuera del catálogo. |
| minor | Un override podía **colapsar dos ids en el mismo código** (colisión de clave de caché y `byCode` ambiguo) o poner un código vacío. | Ambos rechazados nombrando los ids y el código. |
| minor | Sin `context`: una petición en vuelo **no se podía cancelar** y el backoff dormía con `time.Sleep`. El 130 estaba cableado pero sin motor. | `Latest(ctx, ids)`, `LatestCodes(ctx, codes)`, `History(ctx, id, rango)`, `http.NewRequestWithContext`, backoff con `select` sobre `ctx.Done()`, y `ctx.Err()` → **`cancelled`/2**, no `network`/6: interrumpir no es un fallo de red. |

## Verificación del autor

Los tests del verificador y los de la corrección, corridos por mí:

| Test | Resultado |
|---|---|
| `TestLatestFavoritasArrayIncompletoEsAPIChanged` | PASS |
| `TestLatestFavoritasArrayFueraDeOrdenEsAPIChanged` | PASS |
| `TestHistoryOrdenAscendenteEsAPIChanged` | PASS |
| `TestCatalogoOverrideCodigoVacio` | PASS |
| `TestCatalogoOverrideColisiona` | PASS |
| `TestLatestContextoCancelado` (en vuelo, cancela a 50 ms) | PASS, `cancelled`/2 |
| `TestLatestBackoffInterrumpible` (backoff de 10 s, cancela a 50 ms) | PASS, retorna en 0,05 s |

`grep` sobre `doWithRetry` confirma el `select` entre `ctx.Done()` y `time.After`, y
`cancelledError(ctx)` en los dos caminos de salida (reintento y lectura del cuerpo).
`bash scripts/check.sh` → **EXIT=0**, «No vulnerabilities found».

## Nit honesto sobre el historial

El mensaje del commit `015f723` dice «reemplaza un parser escrito a mano», pero respecto de
`dev` ese archivo es nuevo: el parser artesanal nació y murió dentro de esta misma rama. El
efecto neto es el que describe (191 líneas con `yaml.v3`), pero el historial no debe narrar
un reemplazo que no ocurrió en `dev`. Se deja anotado aquí en vez de reescribir la historia
con un force-push.

## Límites que quedan escritos

- Los cortes de conexión **no** se reintentan: `docs/series.md` §7 los menciona, la decisión
  6 de la tarea (normativa) limita el reintento a 5xx/429. El código sigue la tarea; si se
  quiere reintentar cortes, es un cambio de decisión, no un bug.
- `-0.0` se serializa como `0.0` (shopspring normaliza el cero negativo): irrelevante para
  tipos de interés y sin efecto en la igualdad de valor.
- Los redirects se siguen (comportamiento por defecto de `net/http`); no hay credenciales en
  juego.
- El job nocturno contra la API real sigue pendiente: este cliente es su requisito.

## Veredicto

**PASS tras la ronda única de correcciones.** El major que habría producido cuotas
calculadas con un índice que no existía está cerrado; la exactitud decimal, que era el
riesgo de fondo, quedó probada por sondas que yo no habría escrito.
