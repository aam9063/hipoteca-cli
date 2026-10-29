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

## Veredicto

**PASS** con la verificación del autor. Pendiente el verificador independiente formal, que es el
que debe atacar el payload con `--with-impact` y el reintento de eventos.
