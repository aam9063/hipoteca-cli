# OPS-02: review gate en CI, plantillas y linter anti-float

## Objetivo

Cerrar los tres agujeros que quedaron abiertos después de FIN-01 y CORE-01: la guarda
anti-float que se puede burlar, el review gate sin plantilla que lo explique, y el cableado
de la interrupción sin verificación estructural.

## Problema y motivo

El verificador independiente de FIN-01 **burló la guarda anti-float** colocando
`decimal.NewFromFloat(1.5)`, `strconv.ParseFloat("3.14", 64)` y `reflect.ValueOf(1.5).Float()`
dentro de `internal/finance`, y `TestNoFloatInFinance` siguió en verde. La guarda sólo busca
los identificadores `float64`/`float32`; no ve **literales** float ni **llamadas** a APIs que
meten floats.

Se reprodujo hoy en este repositorio: el test pasa con esas tres líneas dentro del paquete.
P-07 es el principio más caro del proyecto; una guarda que se deja burlar con tres líneas
razonables da confianza falsa.

## Alcance

- Dentro:
  - Guarda anti-float ampliada en `internal/finance` (test de AST, sin dependencias nuevas).
  - Plantilla de pull request y plantillas de issue.
  - Test estructural del cableado de la señal en `cmd/hipoteca`: `signal.NotifyContext` y
    `ExecuteContext` presentes, para que el 130 no dependa de que nadie lo borre.
- Fuera:
  - `forbidigo`: no aporta sobre lo que se puede hacer con `go/ast` y añadiría una
    herramienta externa al camino de `make check`.
  - Añadir un comando artificialmente largo para probar el 130 de punta a punta.

## Diseño de la guarda

Sobre los ficheros **no-test** de `internal/finance`, falla ante:

1. Un identificador `float32` o `float64` (lo que ya hacía).
2. Un literal de tipo float (`token.FLOAT`), que es como entra un `1.5` suelto.
3. Un selector cuyo nombre contenga `Float` o `ParseFloat` (`NewFromFloat`, `ParseFloat`,
   `Float64frombits`, `Float64bits`, `Value.Float`, ...).
4. Una conversión a `float32`/`float64` (cubierta por 1, se mantiene).

El mensaje de cada hallazgo da posición y motivo. El propio test vive en un `_test.go`, así
que sus literales no se autoinculpan.

## Superficie de edición (nuestra)

- `internal/finance/nofloat_test.go`
- `cmd/hipoteca/main_test.go`
- `.github/pull_request_template.md`, `.github/ISSUE_TEMPLATE/**`
- `odd/tasks/OPS-02-linter.md`, `odd/reviews/OPS-02.md`

## Riesgo (nuestra)

**Medio.** No cambia comportamiento de producto; cambia la capacidad de detectar un defecto
caro. El riesgo real es una guarda con falsos positivos que moleste y termine desactivada:
por eso se prueba contra los casos legítimos del paquete actual.

## Checklist

- [x] OPS-02.1 Guarda ampliada · Criterio: falla con `decimal.NewFromFloat(1.5)`,
      `strconv.ParseFloat` y `reflect.Value.Float()` dentro del paquete (los tres casos que
      burlaron la versión anterior), y sigue pasando con el código real de `internal/finance`.
- [x] OPS-02.2 Cero falsos positivos · Criterio: el paquete tal como está hoy pasa la guarda
      sin excepciones.
- [x] OPS-02.3 Plantilla de PR con el checklist del review gate · Criterio: menciona
      `make check`, `odd/reviews/<ID>.md` y las reglas duras de `AGENTS.md`.
- [x] OPS-02.4 Plantillas de issue (bug y feature) · Criterio: la de bug pide versión,
      comando y salida observada.
- [x] OPS-02.5 Test estructural del cableado de la señal · Criterio: si alguien quita
      `signal.NotifyContext` o `ExecuteContext` de `main.go`, el test falla.
- [x] OPS-02.6 Verificación · Criterio: `make check` verde y la guarda probada contra los
      tres casos de burla.

## Evidencia de verificación

- Guarda anti-float (RED): con las tres líneas de burla en un fichero no-test del paquete
  (`bypass_repro_scratch.go`, eliminado tras la reproducción), `go test ./internal/finance
  -run TestNoFloatInFinance` falla con 6 hallazgos con posición y motivo: selector
  `NewFromFloat` (13:9), literal `1.5` (13:30), identificador `float64` (15:16), selector
  `ParseFloat` (16:10), selector `Float` (20:9), literal `1.5` (20:25). Exit 1.
- Guarda anti-float (GREEN): sin el fichero de reproducción, el mismo test pasa sobre los
  ficheros no-test actuales (`doc.go`, `finance.go`, `schedule.go`) sin excepciones. Exit 0.
- Cableado de la señal (control negativo): la lógica del detector, ejecutada contra
  `main.go` de juguete en un directorio temporal, detecta la ausencia de `ExecuteContext`,
  la ausencia de ambos, y el cableado completo (`true/true`). El propio `main.go` no se
  tocó (fuera de superficie).
- Cableado de la señal (GREEN): `go test ./cmd/hipoteca -run TestSignalWiringIsPresent`
  pasa contra el `main.go` real.
- `bash scripts/check.sh`: verde (gofmt, go mod tidy, go vet, staticcheck, govulncheck,
  go test).

## Progreso

Implementación completa: guarda ampliada, plantillas de PR e issue, test estructural del
signal wiring. Pendiente de review gate y del informe en `odd/reviews/OPS-02.md`.

## Siguiente paso

Review gate con informe en `odd/reviews/OPS-02.md`.

## Log

### L1 · 2026-10-28 · decisión del usuario

"Cierra cualquier gap y seguimos." Los gaps declarados eran: la guarda anti-float
superficial (demostrada por el verificador de FIN-01 y reproducida hoy), el 130 sin
verificación del cableado, y el review gate sin plantilla.
