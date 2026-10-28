# FIN-01: cuota, cuadro de amortización y tipo 0 %

## Objetivo

Implementar en `internal/finance` el cálculo del sistema francés —cuota, interés mensual,
amortización y cuadro completo— en aritmética exacta, validado contra los casos de
referencia de `testdata/finance/`. Es el corazón del proyecto y la base de FIN-02, FIN-03 y
FIN-04.

## Problema y motivo

Todo lo que la CLI va a decirle a alguien sobre su hipoteca sale de aquí. Un error de un
céntimo por cuota, repetido 300 veces, son 3 € pero un error de convención son miles. Es la
tarea de mayor riesgo del backlog y la única cuyo veredicto depende de números que no
escribimos nosotros.

## Alcance

- Dentro:
  - `Payment(principal, annualRate, months)` — cuota constante, sistema francés.
  - `Schedule(loan)` — cuadro completo con totales, interés mensual redondeado y última
    cuota absorbiendo el residuo.
  - Caso `i = 0` sin división por cero.
  - Validación de entradas con errores centinela.
- Fuera:
  - Revisión, suelo, redondeo de escritura y mixta (FIN-02).
  - Amortización anticipada (FIN-03) y escenarios/comparativa (FIN-04).
  - Linter anti-float (OPS-02), aunque el criterio de "ningún `float64`" ya aplica aquí.

## Restricciones

- P-07: nada de `float64`. Ni en la API ni en el cálculo.
- P-04: el cuadro devuelve los datos que la CLI necesita para mostrar las entradas.
- `internal/finance` es puro: sin red, sin E/S, sin reloj. Sin dependencia de `internal/bde`.
- Las convenciones son las de `docs/calculos.md`. Si un caso de referencia discrepa, **se
  para y se pregunta** (arranque §9): no se toca el caso.

## Decisión de diseño: D-03 (decimal exacto)

El arranque dejaba abierto `shopspring/decimal` frente a `math/big.Rat`. Se resuelve así:

- **Cálculo interno con `math/big.Rat`.** El tipo mensual `TIN/100/12`, la potencia
  `(1+i)^n` y la cuota son racionales exactos; con `big.Rat` no hay aproximación en ningún
  paso. La cuota exacta se redondea half-up a céntimos una sola vez.
- **API pública con `shopspring/decimal`.** Legibilidad para el resto del código y para el
  formateo. La conversión `big.Rat → decimal` ocurre sólo en los bordes, después de
  redondear.

Motivo para no usar `decimal` en el núcleo: `shopspring` toma la precisión de división de
una **variable global mutable** (`decimal.DivisionPrecision`), y este paquete no debe
depender de estado global para que su aritmética sea correcta. Con `big.Rat` la exactitud
es una propiedad del código, no de una configuración.

## API

```go
// Rate is an annual nominal rate expressed in percent, e.g. 3.10 means 3.10 %.
// Amounts and rates are exact decimals; months are integers.

func Payment(principal, annualRate decimal.Decimal, months int) (decimal.Decimal, error)

type Loan struct {
    Principal  decimal.Decimal
    AnnualRate decimal.Decimal // percent, e.g. "3.10"
    Months     int
}

type Row struct {
    Month     int             // 1-based
    Payment   decimal.Decimal
    Interest  decimal.Decimal
    Principal decimal.Decimal
    Balance   decimal.Decimal
}

type Schedule struct {
    Rows          []Row
    TotalInterest decimal.Decimal
    TotalPaid     decimal.Decimal
}

func Amortization(loan Loan) (Schedule, error)
```

Errores centinela: `ErrNegativePrincipal`, `ErrZeroMonths`, `ErrNegativeRate`,
`ErrNegativeMonths`. `internal/errs` los mapeará a `usage`/`internal` en CORE-01.

## Superficie de edición (nuestra)

- `internal/finance/**`
- `go.mod`, `go.sum` (añadir `shopspring/decimal`)

## Riesgo (nuestra)

**Alto.** Ítem 6 de la lista: es dinero, y un fallo de cálculo no lo detecta ningún test que
no sea un caso de referencia independiente. Por eso los casos de SPIKE-02 existen y por eso
esta tarea lleva **verificador independiente** además del review gate.

## Checklist

- [x] FIN-01.1 Test en rojo contra los casos 1-5 de `testdata/finance/` · Criterio: el test
      lee los JSON (no números escritos a mano en el test) y falla antes de la
      implementación. **Rojo observado**: build sin las funciones.
- [x] FIN-01.2 `Payment` con `big.Rat` y redondeo half-up · Criterio: reproduce las cuotas
      de los casos 1 y 5 y el caso 2 (`i = 0`) sin división por cero.
- [x] FIN-01.3 `Amortization` con interés mensual redondeado y última cuota que absorbe el
      residuo · Criterio: capital final exactamente `0.00` en los cinco casos fijos.
- [x] FIN-01.4 Propiedades · Criterio: la suma de amortizaciones iguala el principal; el
      capital nunca es negativo ni crece; `TotalPaid == TotalInterest + Principal`.
      **La invariante del capital fue falsa en un principio**: ver el hallazgo del
      verificador y la corrección de cierre anticipado.
- [x] FIN-01.5 Validación de entradas · Criterio: principal negativo, meses ≤ 0 y tipo
      negativo devuelven el error centinela correspondiente.
- [x] FIN-01.6 Verificación de que no hay `float64` en el paquete · Criterio: test de AST
      que falla si aparece un `float64` o un `float32` (adelanto mínimo de OPS-02, sólo
      como test, sin linter). Límite real documentado: no ve floats escondidos tras otras
      APIs, `interface{}` ni `unsafe`.
- [x] FIN-01.7 Review gate con verificador independiente · Criterio: informe en
      `odd/reviews/FIN-01.md` con el veredicto del verificador. Verificador escalado por el
      ítem de riesgo 6 y aceptado por el plan (`independentVerifier: true`).

## Evidencia de verificación

- **Rojo antes de implementar**: `go test ./internal/finance/` falla con
  `undefined: Payment / Amortization / Loan / Row` (no existía el paquete).
- **Verde**: los cinco casos de referencia, el caso de tipo 0 %, las propiedades, la
  validación de entradas y la guarda anti-float.
- **Contraste fuera de los fixtures** (para descartar sobreajuste): 250000.00 al 4.15 % a
  372 meses contra una implementación independiente → cuota 1195.58, interés total
  194754.99, primera fila 864.58 / 331.00 / 249669.00, última cuota 1194.81, cierre 0.00.
  Idénticos.
- **Verificador independiente** (contexto limpio, sólo lectura, escalado por ítem de riesgo
  6): PASS WITH FIXES. Halló que la cuota redondeada podía liquidar el préstamo antes del
  último mes y emitir filas con saldo y pago negativos (150000.00 al 23.62 % a 480 meses:
  pago de −270.20). Corregido con cierre anticipado; re-verificado por el autor con
  implementación independiente: 479 filas, última cuota 2687.77, interés total 1264107.05,
  cierre 0.00.
- **Rejilla de propiedades**: 245 combinaciones (5 capitales × 7 tipos de 0.01 % a 25 % ×
  7 plazos de 1 a 480 meses); falla en la combinación exacta del hallazgo.
- `bash scripts/check.sh` → exit 0. CI del PR #6, run `40721021576`: `check` en ubuntu,
  macos y windows más `review gate report`, los cuatro en success.
- El residual queda probado como load-bearing: al quitarlo fallan los casos 1, 3, 4 y 5.

## Progreso

**Cerrada.** PR #6 mergeado a `dev`.

## Siguiente paso

Ola 1: `CORE-01` (sobre de error, códigos de salida y `schema`), `CORE-02` (config XDG) y,
tras CORE-02, `BDE-01` (cliente de la API con caché y `series.yaml`). `FIN-02` (revisión,
suelo, redondeo, mixta) ya puede construirse sobre `Payment`/`Amortization`.

## Log

### L1 · 2026-10-28 · decisión del usuario

Ante las tres preguntas de SPIKE-02, el usuario responde "todo sí": aprueba las convenciones
(1 y 4 incluidas), pide regenerar los casos 6-8 con índices reales ya publicados, y autoriza
arrancar FIN-01 con esos casos.

### L2 · 2026-10-28 · decisión técnica

D-03 resuelta: `big.Rat` en el núcleo por exactitud, `shopspring/decimal` en la API pública
por legibilidad. Se descarta `decimal` en el núcleo porque su precisión de división es una
variable global mutable.

### L3 · 2026-10-28 · cierre

PR #6, CI run `40721021576` en verde (los tres `check` y el review gate). El verificador
independiente encontró un defecto real que ni los casos de referencia ni el contraste del
autor detectaron, y su corrección añadió la rejilla de propiedades que lo habría cazado.
Los hashes en `dev` se consultan con `git log`; el registro cita el PR y el run porque el
merge con rebase los reescribe.
