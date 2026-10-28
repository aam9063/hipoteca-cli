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

- [ ] FIN-01.1 Test en rojo contra los casos 1-5 de `testdata/finance/` · Criterio: el test
      lee los JSON (no números escritos a mano en el test) y falla antes de la
      implementación.
- [ ] FIN-01.2 `Payment` con `big.Rat` y redondeo half-up · Criterio: reproduce las cuotas
      de los casos 1 y 5 y el caso 2 (`i = 0`) sin división por cero.
- [ ] FIN-01.3 `Amortization` con interés mensual redondeado y última cuota que absorbe el
      residuo · Criterio: capital final exactamente `0.00` en los cinco casos fijos.
- [ ] FIN-01.4 Propiedades · Criterio: la suma de amortizaciones iguala el principal; el
      capital nunca es negativo ni crece; `TotalPaid == TotalInterest + Principal`.
- [ ] FIN-01.5 Validación de entradas · Criterio: principal negativo, meses ≤ 0 y tipo
      negativo devuelven el error centinela correspondiente.
- [ ] FIN-01.6 Verificación de que no hay `float64` en el paquete · Criterio: test de AST
      que falla si aparece un `float64` o un `float32` (adelanto mínimo de OPS-02, sólo
      como test, sin linter).
- [ ] FIN-01.7 Review gate con verificador independiente · Criterio: informe en
      `odd/reviews/FIN-01.md` con el veredicto del verificador.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Documento y API fijados; implementación no iniciada.

## Siguiente paso

Rama, test en rojo, implementación.

## Log

### L1 · 2026-10-28 · decisión del usuario

Ante las tres preguntas de SPIKE-02, el usuario responde "todo sí": aprueba las convenciones
(1 y 4 incluidas), pide regenerar los casos 6-8 con índices reales ya publicados, y autoriza
arrancar FIN-01 con esos casos.

### L2 · 2026-10-28 · decisión técnica

D-03 resuelta: `big.Rat` en el núcleo por exactitud, `shopspring/decimal` en la API pública
por legibilidad. Se descarta `decimal` en el núcleo porque su precisión de división es una
variable global mutable.
