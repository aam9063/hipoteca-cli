# BDE-01: cliente de la API del Banco de España, catálogo y caché

## Objetivo

Leer los índices oficiales del Banco de España desde la CLI: cliente HTTP con el contrato de
`docs/series.md`, catálogo `series.yaml` embebido con overrides de config, y caché local que
respete la regla de que los meses publicados no cambian.

## Problema y motivo

Es el único paquete que hace red y la fuente de todo dato que después se convierte en dinero.
Los índices llegan como número JSON: si se decodifican como `float64` se rompe P-07 en el
borde, antes de cualquier cálculo. Y el contrato tiene trampas ya documentadas (errores con
HTTP 200, cuerpo array vs objeto, rangos válidos escasos) que un cliente ingenuo no maneja.

## Alcance

- Dentro:
  - `Latest(ids []string)` → último valor de cada índice.
  - `History(id string, rango Rango)` → histórico mensual.
  - Catálogo `series.yaml` con `go:embed` + overrides desde `internal/config`.
  - Caché en `config.CacheDir()`: 24 h para el último valor, histórico cerrado sin caducidad.
  - Mapeo de errores a los códigos del arranque §6.4.
  - Tests contra los trece fixtures de `testdata/bde/` (ningún test toca la red).
- Fuera:
  - Revisión, cálculos y comandos: FIN-02, CMD-01.
  - El job nocturno contra la API real: se añade cuando este cliente exista y funcione.

## Decisiones

1. **`json.Number`, nunca `float64`.** Se decodifica con `decoder.UseNumber()` y se convierte
   a `model.Decimal`. Un `float64` en este paquete es un defecto, y la guarda anti-float de
   `internal/finance` se extiende aquí.
2. **Rangos del contrato, no inventados.** `Rango30M`, `Rango60M`, `RangoMAX`, `RangoAnio(año)`.
   La API rechaza `12M`, `24M`, `36M`, `120M` y años futuros: no se exponen. Un `--range`
   arbitrario de la CLI se resuelve en CMD-01 pidiendo `MAX` o el año y recortando en cliente.
3. **El catálogo manda sobre la respuesta.** El `id` que ve el usuario (`euribor-1y`) sale de
   `series.yaml`; los textos del Banco se usan sólo como metadato. Los overrides de config
   reemplazan el `code` de un `id` existente; un `id` desconocido en config es un error de
   configuración, no un índice nuevo.
4. **La caché es descartable.** Se escribe sin la maquinaria atómica de `internal/config`
   (son datos públicos regenerables) pero **se valida al leer**: un archivo de caché corrupto
   o de otra versión de formato se ignora y se vuelve a pedir a la red. Nunca un error duro
   por caché.
   - Último valor: 24 h.
   - Histórico: los meses ya publicados no cambian, así que la entrada no caduca; se refresca
     sólo si el rango pedido incluye un mes posterior al último guardado.
5. **Errores mapeados según `docs/series.md` §6**: `listaSeries` con `errNum` "no existe" →
   `not_found`/4; cualquier otra forma inesperada o cuerpo no-JSON → `api_changed`/7; timeout,
   5xx y cortes → `upstream_unavailable`/5; 429 → `rate_limited`/5; fallo de red sin caché →
   `network`/6. Un elemento con `errNum` en `favoritas` es `api_changed`/7 si el código sale
   del catálogo, y `not_found`/4 si el usuario forzó ese código.
6. **HTTP acotado**: timeout de 15 s, `User-Agent: hipoteca-cli/<version>`, `Accept-Encoding`
   por defecto del transporte, y como máximo **un** reintento con backoff ante 5xx/429. Sin
   reintentos infinitos ni bucles.

## Superficie de edición (nuestra)

- `internal/bde/**` (menos `series.yaml`, que ya existe)
- `go.mod`, `go.sum` si hiciera falta
- `odd/tasks/BDE-01-cliente.md`, `odd/reviews/BDE-01.md`

## Riesgo (nuestra)

**Alto, ítem 6**: una regresión de parseo o de caché no la detectaría ningún test de cálculo,
porque el dato entra mal desde el principio. Lleva verificador independiente.

## Checklist

- [x] BDE-01.1 Cliente con `favoritas` y `listaSeries` sobre `httptest`, usando los fixtures
      reales como cuerpo de respuesta · Criterio: parsea los trece sin tocar la red.
- [x] BDE-01.2 Valores como `model.Decimal` exactos · Criterio: `3.247` y `-0.272` sobreviven
      sin pasar por `float64`; guarda anti-float en el paquete.
- [x] BDE-01.3 Catálogo embebido + overrides · Criterio: los 14 `id` de `series.yaml`
      resuelven; un override cambia el código; un `id` desconocido da error.
- [x] BDE-01.4 Caché con las dos políticas · Criterio: el último valor caduca a las 24 h; el
      histórico no caduca; la caché corrupta se ignora.
- [x] BDE-01.5 Mapeo de errores · Criterio: los casos de `testdata/bde/error-*` producen los
      códigos 4, 5, 6 y 7 esperados.
- [ ] BDE-01.6 Verificador independiente · Criterio: informe en `odd/reviews/BDE-01.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Test-first contra los trece fixtures: RED observado (24 tests fallando sobre los stubs) y
GREEN tras implementar (`go test ./internal/bde/` en verde, suite completa `go test ./...`
en verde). Pendiente: verificador independiente (BDE-01.6) y `make check` del cierre —
ver Log L2 por el bloqueo de `govulncheck`.

## Siguiente paso

Test en rojo contra los fixtures, implementación, verificación.

## Log

### L1 · 2026-10-28 · decisión del usuario

Tras cerrar CORE-02, se continúa con BDE-01, su dependiente directo y el primer contacto real
con la API del Banco.

### L2 · implementación (BDE-01.1–BDE-01.5)

Implementado test-first (RED observado, luego GREEN):

- `client.go`: `HTTPClient` con `*http.Client` inyectable (15 s, `User-Agent:
  hipoteca-cli/dev`), `Latest`/`LatestCodes`/`History`, decodificación con `json.Number` +
  `UseNumber()` → `model.Decimal`, un único reintento con backoff ante 5xx/429, y la tabla
  de mapeo de errores de docs/series.md §6 (`idioma=es` y `series` viajan por construcción).
- `history.go`: `Rango` (30M/60M/MAX/año; años futuros no se exponen), `Observation` y
  `Series` en orden ascendente (la inversión del orden descendente de la API ocurre una vez,
  en el cliente).
- `catalog.go`: `series.yaml` con `go:embed` y un parser estricto del subconjunto YAML que usa
  el fichero (sin dependencias nuevas), `Lookup`/`IDs`, overrides de `Config.Indices` con
  error que nombra el id desconocido.
- `cache.go`: entradas versionadas (`format_version: 1`) bajo el directorio dado; 24 h para
  el último valor y rangos abiertos; histórico de año cerrado sin caducidad; fichero corrupto,
  ajeno o de otra versión se ignora y refetch; nunca error duro por caché.
- `nofloat_test.go`: la guarda AST de `internal/finance` replicada para este paquete.

Verificación: `go test ./...` verde; `bash scripts/check.sh` llega verde hasta `govulncheck`,
que falla (exit 3) porque la toolchain local (go1.26.1) tiene vulnerabilidades de stdlib
(`crypto/tls`, `crypto/x509`, arregladas en go1.26.2) que este paquete —el primero que hace
HTTPS— hace alcanzables. Comprobado contra el commit base en un worktree temporal:
`govulncheck ./...` en la base sale 0. El arreglo es subir la toolchain (fuera de la superficie
de edición de esta tarea); no es un defecto del código nuevo.

### L3 · 2026-10-28 · ronda de corrección (verificador: PASS WITH FIXES)

Fixes 1–4 del verificador implementados test-first (RED observado en los seis tests nuevos,
GREEN tras implementar; `go test ./internal/bde -run 'TestLatest|TestHistory|TestCatalogo|TestRango'`
en verde):

- Fix 1: `favoritas` valida contra los códigos pedidos — exactamente un elemento por código,
  en orden, con `serie` (o `codigo` si trae `errNum`) igual al pedido; array vacío, corto,
  duplicado o fuera de orden → `api_changed`/7 nombrando lo esperado y lo llegado. Ya no se
  cachea un éxito con datos que faltan.
- Fix 2: `listaSeries` comprueba que `fechas` llega no-creciente por mes **antes** de
  invertir; una respuesta ascendente → `api_changed`/7.
- Fix 3: objeto de error de `favoritas` sobre un código del catálogo → `api_changed`/7;
  `not_found`/4 se reserva para códigos forzados fuera del catálogo.
- Fix 4: overrides rechazan código vacío y colisiones de código entre ids, nombrando el id
  o los dos ids en conflicto.
- Fix 5: `Latest(ctx, ids)`, `LatestCodes(ctx, codes)` e `History(ctx, id, rango)`
  llevan `context.Context`; las peticiones se construyen con
  `http.NewRequestWithContext` y la espera del reintento es interrumpible
  (`select` sobre `ctx.Done()`). Un contexto cancelado sube como
  `cancelled`/2 (arranque §6.4), nunca como `network`/6. Tests:
  `TestLatestContextoCancelado` (en vuelo, vuelve enseguida con cancelled/2) y
  `TestLatestBackoffInterrumpible` (el backoff de 10 s se corta al cancelar).
- `cache_test.go` (superficie aprobada por el usuario en esta ronda): cambio
  mecánico — `context.Background()` en todas las llamadas y, para `Latest`, un
  cuerpo de un solo elemento con el valor del fixture real (3.247 en 2026-09):
  la cardinalidad estricta de §3.1 exige un elemento por código pedido y así la
  clave de caché sigue siendo "D_1NBAF472". Ninguna aserción de caché se debilita.
- Resultado: `go test ./internal/bde` en verde; `bash scripts/check.sh` en verde
  (exit 0), incluido `govulncheck` tras subir la toolchain (ver L2).
