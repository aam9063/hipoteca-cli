# SPIKE-02: fórmulas, convenciones y casos de referencia

## Objetivo

Fijar por escrito las fórmulas y convenciones de cálculo de la CLI, y producir los casos
de referencia **calculados fuera del proyecto** contra los que FIN-01 a FIN-04 probarán
`internal/finance`. Sin este documento, FIN-01 sería un agente comprobando su propio código
contra sus propias suposiciones.

## Problema y motivo

Cinco tareas de la Ola 1-3 dependen de esto: FIN-01 (cuota y cuadro), FIN-02 (revisión,
suelo, redondeo, mixta), FIN-03 (amortización anticipada), FIN-04 (escenarios) y CMD-01.
El arranque §14 lo declara dependencia de FIN-01 y §9 prohíbe que un agente "arregle" un
caso de referencia para que pase un test.

El riesgo real: una CLI de hipotecas que se equivoca en un céntimo por cuota durante 25
años se equivoca en miles de euros, y el usuario decide con ese número. El caso de
referencia tiene que venir de **otra implementación**, no de la nuestra.

## Alcance

- Dentro:
  - `docs/calculos.md`: fórmulas, convenciones de tipo, redondeo y residual, con las
    decisiones cerradas y su justificación.
  - `testdata/finance/`: 10 casos en JSON, con entradas, salidas esperadas y metadatos de
    origen (versiones, método, hash del generador).
  - `testdata/finance/generate.py`: el generador reproducible, versionado como
    documentación del origen. No es código de producto y no entra en el binario.
- Fuera:
  - Implementar `internal/finance` (FIN-01 a FIN-04).
  - Validar contra recibos bancarios reales (H-03); esto es el sustituto acordado con el
    usuario, no su reemplazo.

## Restricciones

- P-07: todo en decimal exacto, redondeo half-up a céntimos. Nada de `float64` en el
  producto.
- P-04: cada caso lleva sus entradas completas, para que el número se pueda recalcular a
  mano.
- §9: el generador es independiente del código Go y los casos no se tocan para que un test
  pase. Si un caso discrepa, se para y se pregunta.

## Convenciones (las decisiones de esta tarea)

1. **Tipo mensual nominal**: `i = TIN / 100 / 12`. Es el estándar del sistema francés en
   las escrituras españolas y el valor por defecto de `interest_convention`. La conversión
   de tipo efectivo anual (`(1+TIN/100)^(1/12)-1`) **no** se soporta en v1: si se necesita,
   es otro valor de la misma opción, en su PR.
2. **Cuota**: `C = P · i / (1 − (1 + i)^−n)`; con `i = 0`, `C = P / n`. `n` son los meses
   restantes.
3. **Redondeo de la cuota**: half-up a céntimos (`payment_rounding = "half-up-cents"`).
   La cuota es constante y redondeada; **la última cuota absorbe el residuo** para que el
   capital final sea exactamente `0.00`. Es lo que hace un recibo real y lo que evita
   arrastrar un céntimo durante 300 cuotas.
4. **Interés del mes**: `P · i` redondeado half-up a céntimos **antes** de calcular la
   amortización (`amortización = cuota − interés`). Se documenta porque la alternativa
   (arrastrar el interés sin redondear) da totales que difieren en céntimos y hay que
   elegir una.
5. **Revisión**: el TIN nuevo es `índice + diferencial`; luego se aplica el **suelo**
   (`max(TIN, suelo)`) y después el **redondeo de la escritura** (por ejemplo `1/8`:
   múltiplo de 0.125 más próximo, half-up). Al revisar se recalcula la cuota con el capital
   pendiente y los meses restantes, aplicando las mismas reglas.
6. **Tolerancia de cierre**: el capital final del cuadro es exactamente `0.00` por diseño
   (regla 3). La tolerancia explícita de los tests es `±0.01` sólo para comparar con
   terceros, nunca para justificar un residuo en nuestra propia simulación.

## Los 10 casos

| # | Caso | Qué fija |
|---|---|---|
| 1 | Fijo 150000.00 al 3.10 %, 300 meses | Caso base: cuota, cuadro completo, totales |
| 2 | Fijo 120000.00 al 0.00 %, 240 meses | Rama `i = 0`: cuota = capital/meses, sin intereses |
| 3 | Fijo 90000.00 al 0.50 %, 180 meses | Tipo bajo: el redondeo a céntimos pesa |
| 4 | Fijo 60000.00 al 6.25 %, 60 meses | Plazo corto, tipo alto: cuotas grandes |
| 5 | Fijo 183450.75 al 2.37 %, 347 meses | Importes y plazos "feos": cuota con muchos decimales |
| 6 | Mixta: 12 meses al 1.25 %, luego revisiones anuales | Cambio de tramo y recálculo |
| 7 | Revisión con suelo: índice 1.500 + 0.890, suelo 3.000 | El suelo muerde y se aplica antes del redondeo |
| 8 | Redondeo 1/8: índice 2.211 + 0.890 = 3.101 | Redondeo de escritura a múltiplo de 0.125 → 3.125 |
| 9 | Amortización 10000.00 en el mes 60, `reduce-payment` | Mismo plazo, cuota menor, ahorro de intereses |
| 10 | Amortización 10000.00 en el mes 60, `reduce-term` | Misma cuota, menos meses |

Cada JSON lleva: `id`, `description`, `inputs`, `expected` (cuota, total de intereses,
total pagado, primera y última fila del cuadro, suma de amortizaciones y capital final) y
`source` (versiones de Python/NumPy/numpy-financial, métodos usados, sha256 del generador,
fecha de generación).

## Superficie de edición (nuestra)

- `docs/calculos.md`
- `testdata/finance/**`
- `odd/tasks/SPIKE-02-calculos.md`, `odd/reviews/SPIKE-02.md`

## Riesgo (nuestra)

**Medio-alto.** No hay código de producto, pero estos números se convierten en el patrón
contra el que se valida todo el dinero del proyecto: un caso mal calculado es peor que
ninguno, porque da confianza falsa. Mitigación: doble método independiente (simulación
exacta con `Decimal` en Python y contraste de la cuota con `numpy_financial.pmt`), y
revisión del usuario antes de fijarlos.

## Checklist

- [ ] SPIKE-02.1 `docs/calculos.md` con las seis convenciones, fórmulas y un ejemplo
      resuelto a mano · Criterio: alguien puede recalcular el caso 1 con una calculadora.
- [ ] SPIKE-02.2 Generador reproducible que simule los 10 casos con `Decimal` (half-up) y
      contraste la cuota con `numpy_financial.pmt` · Criterio: si los dos métodos discrepan
      en más de un céntimo, el generador falla en vez de escribir el caso.
- [ ] SPIKE-02.3 Los 10 JSON en `testdata/finance/` con sus metadatos de origen ·
      Criterio: cada archivo lleva versiones, métodos y sha256 del generador.
- [ ] SPIKE-02.4 Verificación independiente del resultado · Criterio: re-ejecutar el
      generador produce los mismos archivos (deterministas) y el cross-check pasa.
- [ ] SPIKE-02.5 Revisión del usuario de documento y casos · Criterio: el usuario aprueba
      la tabla de casos antes del merge.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Convenciones y casos especificados; producción delegada.

## Siguiente paso

Generar, verificar determinismo, PR y pedir la revisión del usuario antes del merge.

## Log

### L1 · 2026-10-28 · decisión del usuario

Ante la pregunta de cómo obtener los casos de referencia, el usuario elige la opción 2:
"Genero los casos con numpy-financial y vos los revisás". Es decir: sin recibos reales
(H-03) por ahora, casos generados con una implementación independiente y revisados por el
usuario antes de fijarlos.
