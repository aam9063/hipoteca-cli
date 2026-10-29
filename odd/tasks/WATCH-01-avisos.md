# WATCH-01: vigilancias y aviso de índice publicado

## Objetivo

Avisar cuando el Banco de España publica un mes nuevo del índice que sigue una hipoteca, con un
evento `index.published` y un sink configurable. Sin datos del préstamo por defecto.

## Problema y motivo

La revisión ocurre una vez al año, pero el índice se publica cada mes. El usuario quiere
enterarse cuando sale el dato, no acordarse de consultarlo. Es la única funcionalidad del
arranque que **empuja** en vez de responder.

## Alcance

- Dentro:
  - `watch add index <índice> [--sink <nombre>] [--with-impact]`, `watch list`, `watch remove`.
  - `watch check` (una pasada) y `watch run` (bucle, intervalo mínimo 1 h, por defecto 12 h).
  - Detección del mes nuevo comparando con el último ya visto en la base local.
  - Sink **webhook** configurable (un POST JSON a una URL de la config) como único tipo en v1.
  - `--with-impact`: añade al aviso el cambio estimado de cuota **del préstamo**, con la
    advertencia de que implica enviar un dato del préstamo al sink.
- Fuera:
  - `watch service install/uninstall/status` (WATCH-02) y sinks de teléfono específicos.
  - Notificaciones con datos del préstamo por defecto: **nunca**.

## Decisiones

1. **Estado en SQLite** (`modernc.org/sqlite`, sin CGO) en `config.DataPath()`, como fija el
   arranque §8.1: dos tablas, vigilancias y últimos meses vistos. La config TOML sigue siendo
   para los datos del usuario; la base para el estado de ejecución.
2. **Un evento por mes nuevo, ni cero ni dos**: la clave es `(índice, año-mes)` y la inserción es
   idempotente. Repetir `watch check` no duplica avisos.
3. **Sin `--with-impact` el aviso no contiene ni un dato del préstamo**: sólo índice, mes, valor,
   serie y la atribución. Con `--with-impact` la CLI **avisa al configurarlo** de que ese dato
   sale de la máquina (P-03) y el aviso lo incluye marcado.
4. **La atribución viaja en el aviso**: un POST a un webhook ajeno debe llevar el texto del
   Banco y la fecha de actualización.
5. **Sin red o sin dato nuevo no es un error**: `check` devuelve `events: []` y termina con 0.
   Un fallo del sink **sí** es error visible (código 5), pero no borra el evento ya registrado:
   se reintenta en la siguiente pasada.

## Superficie de edición (nuestra)

- `internal/store/**`, `internal/watch/**`
- `internal/app/watch.go`, `internal/cli/watch.go`, `internal/cli/root.go`
- `internal/config/config.go` (sink), `go.mod`, `go.sum`
- `odd/tasks/WATCH-01-avisos.md`, `odd/reviews/WATCH-01.md`

## Riesgo (nuestra)

**Medio-alto.** Ítem 1 (estado persistente que no se puede reconstruir del todo) y ítem 5
(dependencia nueva). Sin verificador independiente no se mergea: es la primera pieza que **saca
datos de la máquina** si el usuario lo pide.

## Checklist

- [ ] WATCH-01.1 `watch add/list/remove` sobre SQLite con round-trip.
- [ ] WATCH-01.2 `check` detecta el mes nuevo y produce **exactamente un** evento; repetirlo no
      duplica.
- [ ] WATCH-01.3 Aviso sin `--with-impact` sin ningún dato del préstamo (test que falla si
      aparece el capital o la cuota).
- [ ] WATCH-01.4 `--with-impact` exige configuración explícita y el aviso se marca.
- [ ] WATCH-01.5 Sink webhook con `httptest`: atribución presente, fallo del sink → código 5 y
      evento conservado.
- [ ] WATCH-01.6 Verificador independiente · Criterio: informe en `odd/reviews/WATCH-01.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Decisiones fijadas; implementación no iniciada.

## Siguiente paso

Test en rojo, implementación, verificación.

## Log

### L1 · 2026-10-29 · decisión del usuario

«watch»: seguir con WATCH-01 tras cerrar FIN-04.

### L2 · 2026-10-29 · corrección del verificador (PASS WITH FIXES)

Cuatro hallazgos corregidos:

1. Bloqueo P-03: el webhook ya no sigue redirecciones (`CheckRedirect` →
   `http.ErrUseLastResponse`); una 3xx es fallo del sink (código 5) con el
   evento a salvo. Test: un sink con 307 a otro host deja el segundo host sin
   recibir nada.
2. El contexto del llamador llega hasta el sink (`deliver(ctx, ...)`);
   una entrega en vuelo aborta con cancelled/2.
3. Guardia de esquema: `PRAGMA user_version` = 1; versión distinta o un
   SQLite ajeno (tablas, user_version 0) se rechazan nombrando fichero y
   versión; nunca se adoptan en silencio.
4. Los mensajes de error del webhook censuran el userinfo de la URL
   (`redactedURL`) y el detalle de transporte va sin la URL
   (`transportCause`).

Más la sonda `TestReintentoSinWithImpactNoSacaDatosDelPrestamo` (el reintento
de una vigilancia sin --with-impact no lleva impacto ni dato del préstamo,
aunque haya calculador conectado). `scripts/check.sh` en verde.
