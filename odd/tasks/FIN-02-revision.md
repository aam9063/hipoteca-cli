# FIN-02: revisión de tipo, suelo, redondeo de escritura y mixta

## Objetivo

Calcular una revisión real: qué TIN resulta de `índice + diferencial`, cómo lo afectan el
suelo y el redondeo de la escritura, qué cuota sale con el capital pendiente y los meses
que quedan, y cómo se encadena todo eso en una hipoteca mixta.

## Problema y motivo

Es el cálculo que el usuario pregunta de verdad: «¿cuánto me cambia la cuota en enero?». Y es
donde las escrituras españolas difieren entre sí: unas redondean el tipo a 1/8, otras a 1/4,
otras no redondean; unas tienen suelo, otras no. Suponer cualquiera de esas cosas da una
cuota distinta de la del banco. El orden también importa: el suelo se aplica **antes** del
redondeo, si no el redondeo puede anularlo.

## Alcance

- Dentro:
  - `NewRate(index, spread, floor, rounding)` → TIN aplicable, con el orden
    `índice + diferencial → suelo → redondeo de escritura`.
  - `ReferenceMonth(revision, offsetMonths)` → año-mes del índice que corresponde.
  - `CompoundSchedule(loan, periods)` → cuadro de una mixta: tramos con su TIN y su número
    de meses, reutilizando `Payment`/`Amortization` para cada tramo.
  - Validación contra los casos 6, 7 y 8 de `testdata/finance/` (índices reales del Banco).
- Fuera:
  - Amortización anticipada (FIN-03), escenarios (FIN-04) y los comandos (CMD-01).

## Decisiones

1. **Orden normativo**: `bruto = índice + diferencial`; `conSuelo = max(bruto, suelo)`;
   `final = redondeoEscritura(conSuelo)`. Cada paso queda en la salida (P-04) para que la CLI
   pueda mostrar el porqué de la cifra.
2. **Redondeos de escritura soportados**: `none`, `1/8` (0.125), `1/4` (0.25) y `0.05`.
   Half-up al múltiplo. Un valor no soportado es error de configuración, no un redondeo
   silencioso.
3. **Sin suelo significa sin suelo**: el campo vacío no equivale a `0`. Un préstamo sin suelo
   declarado no aplica `max(..., 0)`, porque eso convertiría un índice negativo en cero y
   cambiaría la cuota sin que la escritura lo diga.
4. **El mes de referencia es aritmética de meses, no de días**: `revision - offsetMonths`,
   normalizado a año-mes. Ninguna zona horaria ni hora del día entra en la decisión.
5. **La mixta es una lista de tramos**, no un caso especial: el tramo inicial y cada revisión
   aportan TIN y meses; el cuadro se compone reutilizando el mismo núcleo exacto. Si los
   tramos no cubren el plazo total, es error.
6. `internal/finance` sigue puro: sin red, sin reloj, sin E/S. El índice llega como dato.

## Superficie de edición (nuestra)

- `internal/finance/**`
- `odd/tasks/FIN-02-revision.md`, `odd/reviews/FIN-02.md`

## Riesgo (nuestra)

**Alto**, ítem 6: es dinero y el orden suelo/redondeo no lo detecta ningún test que no sea un
caso de referencia. Lleva verificador independiente.

## Checklist

- [ ] FIN-02.1 `NewRate` con el orden normativo · Criterio: reproduce los TIN de los casos 7
      (suelo 3.000 sobre 2.390) y 8 (3.101 → 3.125 con 1/8), y los siete del caso 6.
- [ ] FIN-02.2 Redondeos `none`, `1/8`, `1/4`, `0.05` · Criterio: half-up al múltiplo, con
      casos en el punto medio y un valor no soportado que da error.
- [ ] FIN-02.3 Sin suelo no es suelo cero · Criterio: un índice negativo con suelo vacío da
      un TIN negativo, no 0.
- [ ] FIN-02.4 `ReferenceMonth` · Criterio: enero con offset 2 → noviembre anterior; el
      cambio de año y la fecha invertida se comportan bien.
- [ ] FIN-02.5 `CompoundSchedule` · Criterio: reproduce el cuadro completo de los casos 6, 7
      y 8 (cuotas de cada tramo, interés total, cierre a 0.00), y falla si los tramos no
      cubren el plazo.
- [ ] FIN-02.6 Verificador independiente · Criterio: informe en `odd/reviews/FIN-02.md`.

## Evidencia de verificación

- **Verificador independiente: PASS**, cero blockers y cero majors. Orden suelo→redondeo
  demostrado con un caso que lo distingue, redondeo simétrico en los cuatro modos y ambos
  signos, TIN −5 % cerrando exacto, `CompoundSchedule` coincidente fila a fila con el núcleo
  encadenado, `ReferenceMonth` bajo cuatro zonas horarias.
- **Autor**: cuadro con TIN −0,30 % a 120 meses calculado con implementación independiente →
  cuota 820,79, intereses −1.504,98, cierre 0,00, idéntico. Los diez casos de referencia con
  `sha256` iguales a `dev`.
- **Esta ronda**: tipos negativos soportados en el núcleo, redondeo half-up lejos de cero,
  `ErrUnpayableSchedule` para cuadros sin progreso, y la documentación obsoleta corregida.
- `bash scripts/check.sh` → exit 0.

## Progreso

**Cerrada.** Detalle en `odd/reviews/FIN-02.md`.

## Siguiente paso

`CMD-01` (`index list/show/latest`, `revision`, `schedule`, `calc`) puede construirse sobre
`internal/bde`, `internal/finance` y el contrato de salida. Después `FIN-03` y `FIN-04`.

## Log

### L1 · 2026-10-28 · decisión del usuario

Tras cerrar BDE-01, se continúa con FIN-02: el cliente ya trae índices reales y el núcleo ya
sabe calcular cuotas.

### L2 · 2026-10-28 · tipos negativos y redondeo simétrico

Se cierra la contradicción de la decisión 3: `Payment` y `Amortization` aceptan un TIN
negativo (se elimina `ErrNegativeRate`; sin suelo, índice −0.5 % + diferencial 0.2 % da
TIN −0.3 % y su cuadro cierra a 0.00 exacto). Ambos redondeos (`roundHalfUpCents` y
`roundRateHalfUpToMultiple`) pasan a half-up lejos de cero, simétrico para ambos signos;
los diez casos de referencia quedan byte-idénticos. `scripts/check.sh` en verde.
