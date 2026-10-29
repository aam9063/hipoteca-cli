# Review gate · WATCH-01: vigilancias y aviso de índice publicado

## Cambio bajo revisión

- **Tarea:** `odd/tasks/WATCH-01-avisos.md`. Rama `hc/watch-01-avisos` (base `dev`).
- **Riesgo declarado:** ítem 1 (estado persistente) e ítem 5 (dependencia nueva). Además es la
  primera pieza que **puede sacar datos de la máquina** si el usuario lo pide.

## Lo que entra

`watch add index <id> [--sink] [--with-impact]`, `list`, `remove`, `check` y `run` (intervalo
mínimo 1 h, por defecto 12 h), con estado en SQLite (`modernc.org/sqlite` v1.61.0, sin CGO) en
la ruta de datos y disciplina del config: directorio `0700`, archivo `0600`.

Esquema: `watches(id, kind, target, sink, with_impact, created_at, UNIQUE(kind,target))` y
`seen_months(watch_id, month, value, seen_at, delivered_at, UNIQUE(watch_id,month))`.
`delivered_at NULL` significa pendiente de reenvío.

## La decisión que define la tarea

**Sin `--with-impact`, el aviso no lleva ni un dato del préstamo.** El payload es índice, mes,
valor, serie y atribución:

```json
{"event":"index.published","index":{"id":"euribor-1y","series":"D_1NBAF472"},
 "month":"2026-10","value":"2.211","attribution":{...}}
```

Con `--with-impact` se añade un bloque `impact` marcado `estimated` con una nota que dice que es
una estimación y no un dato del banco, más el aviso P-03 de que ese dato sale de la máquina. El
test serializa el aviso completo y **falla si aparecen `principal`, `payment`, `"impact"`,
`loan`, `casa`, `180000` o `742.18`** sin la bandera.

## Verificación del autor (binario, `HOME` temporal)

| Prueba | Resultado |
|---|---|
| `watch add` sin sink declarado | `usage`/2 con hint que muestra el TOML a escribir |
| config sin `schema_version` | rechazada con mensaje: la validación de CORE-02 aplica |
| `watch add … --sink local` con sink declarado | exit 0 |
| `watch check` con el sink caído | **exit 5** (`upstream_unavailable`) con el mensaje del sink, y el evento queda registrado para reintento |
| `watch check` sin vigilancias | `{"events":[]}`, exit 0 |
| `watch list` | la vigilancia con su id, sink y `with_impact` |

`bash scripts/check.sh` → **EXIT=0**.

## Decisiones interpretadas que conviene mirar

- **Decisión 5, «sin red no es un error»**: el worker lo leyó como «sin dato nuevo → `events: []`
  y exit 0», y **deja los fallos del Banco como sus códigos** (5/6/7). Es la lectura correcta:
  devolver 0 con la API caída haría creer que no hay novedades cuando en realidad no se pudo
  mirar. Mi redacción en el documento era ambigua; queda así.
- **`watch run`** reporta cada fallo de pasada por stderr y sigue. WATCH-02 puede querer revisar
  esa política para el servicio.
- Los timestamps se guardan como RFC3339Nano: si otra herramienta lee `hipoteca.db`, debe usar
  ese formato.

## Límite declarado

No hay `hipoteca config` todavía, así que **el sink se declara editando el TOML a mano**. El
`hint` de `watch add` muestra exactamente qué escribir. Es un hueco real de usabilidad que
corresponde a un comando de config, no a esta tarea.

## Verificador independiente: PASS WITH FIXES

Revisó con probes propios, incluido un servidor que responde redirecciones. Confirmó:

- **La privacidad por defecto resiste incluso en los reintentos**: sin `--with-impact` el payload
  no lleva ni el id del préstamo, ni el pago, ni el bloque `impact`, ni siquiera con `ImpactFor`
  cableado.
- **Un evento por mes**: repetir el check da 0, un valor revisado da 0, un mes nuevo da exactamente
  1. Dos vigilancias sobre el mismo índice son imposibles por diseño (`UNIQUE(kind,target)`).
- **El sink caído conserva el evento** y lo reentrega una sola vez.
- Timeout de 15 s verificado con un servidor lento; `ftp://` y `file://` rechazados; sin cabeceras
  de credenciales.

### Blocker de seguridad encontrado: el redirect reenviaba los datos

Un sink que responde **307/308** hacía que Go **re-POSTeara el payload completo** —con `loan` y
`payment`— **a otro host**, y `Deliver` devolvía éxito. Eso rompe la garantía de la tarea: sólo el
sink que el usuario configuró recibe datos, y encima la CLI decía que todo había salido bien.

Corregido con `CheckRedirect` → `http.ErrUseLastResponse`: un 3xx es la respuesta final, no es 2xx,
da `upstream_unavailable`/5 y el evento queda para reintento. El test nuevo levanta dos servidores
y asserta que **el segundo recibe cero bytes**.

### Los otros tres

| Hallazgo | Corrección |
|---|---|
| **Major**: `deliver` usaba `context.Background()`, así que `Ctrl+C` no cortaba una entrega en curso | El contexto del llamador baja hasta `Deliver`; `ctx.Err()` mapea a `cancelled`/2 |
| **Minor**: ninguna base externa se rechazaba — una SQLite ajena o de esquema futuro se adoptaba en silencio | `PRAGMA user_version`: la 1 se acepta, una mayor o una base con tablas y versión 0 se rechazan nombrando el archivo |
| **Minor**: la URL del sink se ecoaba en los errores, incluida `https://user:token@host` | `redactedURL` (usuario → `redacted`, host y ruta se mantienen) y el detalle del transporte se desenvuelve para no arrastrar la URL |

## Verificación del autor sobre la corrección

`bash scripts/check.sh` → **EXIT=0**. Los seis tests de la ronda en verde:
`TestWebhookNoSigueRedireccionesConElPayload`, `TestWebhookContextoCanceladoAborta`,
`TestWebhookErroresConLaURLCensurada`, `TestOpenFijaLaVersionDeEsquemaEnBaseNueva`,
`TestOpenRechazaVersionDeEsquemaMasNueva` y `TestOpenRechazaSQLiteAjenoSinVersion`.

## Veredicto

**PASS tras la ronda única de correcciones.** El aviso no lleva datos del préstamo sin pedirlo
explícitamente, ningún host distinto del configurado los recibe, y una base ajena no se adopta en
silencio.
