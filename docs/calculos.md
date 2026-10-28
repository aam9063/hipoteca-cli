# Cálculos: fórmulas, convenciones y casos de referencia

Resultado de SPIKE-02. Este documento fija las fórmulas y convenciones de cálculo de la
CLI y describe los casos de referencia de `testdata/finance/`, calculados **fuera del
proyecto** con dos métodos independientes. FIN-01 a FIN-04 prueban `internal/finance`
contra estos casos; ningún agente puede "arreglar" un caso para que pase un test
(arranque §9).

- Generador: `testdata/finance/generate.py` (Python + `decimal.Decimal`, contraste con
  `numpy-financial`). No es código de producto y no entra en el binario.
- Origen y justificación de los casos: `testdata/finance/README.md`.
- Los casos 6 a 8 usan valores **reales publicados** del Euríbor a 1 año (Banco de
  España, serie `D_1NBAF472`, meses de noviembre de 2019 a 2025): ningún índice es
  hipotético. Ver §3.1.

---

## 1. Convenciones (las decisiones de SPIKE-02)

### 1.1 Tipo mensual nominal

```
i = TIN / 100 / 12
```

Es el estándar del sistema francés en las escrituras españolas y el valor por defecto de
`interest_convention` (`monthly-nominal`). Justificación: es lo que hacen las escrituras y
los recibos reales; separarse de ello haría que cada cifra difiriera del banco sin motivo.

**No soportado en v1:** la conversión de tipo efectivo anual a mensual
(`(1 + TIN/100)^(1/12) − 1`). Si algún día hace falta, es otro valor de la misma opción
`interest_convention`, en su PR aparte.

### 1.2 Cuota (sistema francés)

```
C = P · i / (1 − (1 + i)^−n)
```

con `P` el capital pendiente y `n` los meses restantes. Con `i = 0`: `C = P / n` (sin
división por cero; caso 2). Justificación: es la fórmula de la escritura y la que
reproducen las hojas de cálculo (función PAGO) y `numpy_financial.pmt`.

### 1.3 Redondeo de la cuota

Half-up a céntimos (`payment_rounding = "half-up-cents"`), entendido como **half-up lejos
de cero y simétrico para ambos signos**: `0.005` → `0.01` y `−0.005` → `−0.01`. La cuota se
calcula con precisión completa y se redondea **una sola vez**; a partir de ahí es constante. **La
última cuota absorbe el residuo** para que el capital final sea exactamente `0.00`.

Justificación: es lo que hace un recibo real (la última cuota casi nunca coincide al
céntimo con las anteriores) y evita arrastrar un céntimo de diferencia durante 300
cuotas. Por diseño, el capital final del cuadro es exactamente `0.00`: la tolerancia
explícita de `±0.01` existe **solo** para comparar con terceros (otros simuladores, el
banco), nunca para justificar un residuo en nuestra propia simulación.

**Consecuencia en tipos extremos: el plazo puede acortarse.** Redondear half-up puede
dejar la cuota hasta medio céntimo por encima de la cuota exacta. Con combinaciones
extremas de tipo y plazo (tipos de dos dígitos a cientos de meses, inalcanzables en una
hipoteca española), ese excedente acumulado liquida el préstamo antes del mes `n`.
Cuando la cuota redondeada superaría capital pendiente + interés, esa mensualidad pasa a
ser la última: paga exactamente capital + interés, el capital queda en `0.00` y el cuadro
termina antes de `n` filas (p. ej. 150000.00 al 23.62 % a 480 meses cierra en el mes 479,
con cuota final 2687.77). El préstamo se cierra siempre exactamente: el cuadro simplemente
acaba unas filas antes. En tipos de hipoteca (≤ 7 %) el cuadro tiene siempre `n` filas.

### 1.4 Interés del mes

```
interés_mes = redondeo_half_up_a_céntimos(P · i)
amortización = C − interés_mes
P_nuevo = P − amortización
```

El interés se redondea a céntimos **antes** de calcular la amortización. Justificación:
la alternativa (arrastrar el interés sin redondear y redondear solo al final) da totales
que difieren en céntimos y hay que elegir una; redondear cada mes es lo más próximo a un
recibo real, donde cada apunte es un importe con céntimos.

### 1.5 Revisión, suelo y redondeo de escritura

**Los tipos resultantes pueden ser negativos.** Una hipoteca sin suelo con un índice
negativo y un diferencial pequeño produce un TIN negativo (`−0.481 + 0.200 = −0.281`). Ese
caso está soportado y es el de las hipotecas sin suelo de los años de Euríbor negativo: el
interés mensual es negativo, la amortización supera a la cuota y el préstamo se cierra algo
antes, con el cuadro terminando en `0.00` igual que cualquier otro. **No** se recorta el TIN
a cero: hacerlo inventaría un tipo que la escritura no dice (P-06).

El redondeo de escritura del tipo usa la misma regla que la cuota: half-up **lejos de
cero**, simétrico. `−3.0625` con redondeo `1/8` da `−3.125`, no `−3.000`.

En cada revisión:

1. `TIN bruto = índice(mes de referencia) + diferencial`
2. **Suelo**: `TIN = max(TIN bruto, suelo)` (si hay suelo)
3. **Redondeo de escritura** (`rate_rounding`): por ejemplo `1/8` → múltiplo de 0.125 más
   próximo, half-up. El suelo se aplica **antes** del redondeo (caso 7).
4. La cuota se **recalcula** con el capital pendiente y los meses restantes, aplicando las
   convenciones 1.2–1.4.

La amortización anticipada usa la misma maquinaria: comisión
`= importe · early_repayment_fee_pct / 100`; en `reduce-payment` se recalcula la cuota con
el mismo plazo restante; en `reduce-term` se mantiene la cuota y el plazo es el número de
meses que tarda en cerrarse (la última cuota absorbe el residuo).

### 1.6 Tolerancia de cierre

El capital final del cuadro es exactamente `0.00` por diseño (convención 1.3). La
tolerancia de `±0.01` de los tests es solo de contraste con terceros. Ver convención 1.3.

---

## 2. Caso 1 resuelto a mano (verificable con calculadora)

Entradas: capital `150000.00`, TIN `3.10 %`, 300 meses.

**Tipo mensual:** `i = 3.10 / 100 / 12 = 0.0025833333…` (el 3 se repite).

**Cuota:**

```
C = 150000 · 0.0025833333… / (1 − (1.0025833333…)^−300)
  = 387.50 / (1 − 0.4611…)            [el denominador da 0.5388…]
  = 719.143281…                        → redondeo half-up → 719.14
```

**Mes 1:**

```
interés      = 150000 · 0.0025833333… = 387.50        (exacto)
amortización = 719.14 − 387.50        = 331.64
capital pendiente = 150000.00 − 331.64 = 149668.36
```

**Última cuota (mes 300):** el capital pendiente antes de la última cuota es `718.84`,
el interés de ese mes es `1.86`, y la cuota final **no** es 719.14 sino que absorbe el
residuo: `720.70 = 718.84 + 1.86`. Capital final: exactamente `0.00`.

**Totales:** intereses `65743.56`; total pagado `215743.56 = 65743.56 + 150000.00`.

## 3. La tabla de los diez casos

| # | Caso | Entradas | Cuota inicial | Cuota tras el cambio | Interés total |
|---|---|---|---|---|---|
| 1 | Fijo base | 150000.00 · 3.10 % · 300 m | 719.14 | 719.14 (última: 720.70) | 65743.56 |
| 2 | Tipo 0 % | 120000.00 · 0.00 % · 240 m | 500.00 | 500.00 | 0.00 |
| 3 | Tipo bajo | 90000.00 · 0.50 % · 180 m | 519.09 | 519.09 | 3435.95 |
| 4 | Plazo corto, tipo alto | 60000.00 · 6.25 % · 60 m | 1166.96 | 1166.96 | 10017.30 |
| 5 | Importes feos | 183450.75 · 2.37 % · 347 m | 730.87 | 730.87 | 70160.13 |
| 6 | Mixta 12 m | 150000.00 · 1.25 % 12 m → 7 revisiones anuales, Euríbor real (nov) + 0.890 · 96 m | 1642.74 | 1606.79 (TIN 0.618) | 9660.61 |
| 7 | Suelo | 150000.00 · 3.500 % 12 m → Euríbor real (nov) + 0.500, suelo 3.000 · 96 m | 1793.71 | 1763.47 (TIN 3.000, suelo) | 20702.07 |
| 8 | Redondeo 1/8 | 150000.00 · 3.100 % 12 m → Euríbor real (nov) + 0.890, redondeo 1/8 · 96 m | 1766.26 | 1621.28 (TIN 0.625) | 12315.87 |
| 9 | Amortiza, reduce-payment | como caso 1; amortiza 10000.00 tras la cuota 60, comisión 0 % | 719.14 | 663.18 | 62312.38 (ahorro 3431.18) |
| 10 | Amortiza, reduce-term | como caso 1; amortiza 10000.00 tras la cuota 60, comisión 0 % | 719.14 | 719.14 (última: 694.82; plazo 275 m, 25 menos) | 57739.18 (ahorro 8004.38) |

Notas de la tabla:

- En los casos 6 a 8 la cuota inicial es la del tramo inicial y la "cuota tras el cambio"
  la de la primera revisión (mes 13, TIN con el Euríbor de noviembre 2019). Los tramos
  completos del caso 6 están en el JSON (`expected.revisions`): TIN 0.618 → 1606.79,
  0.409 → 1596.67, 0.403 → 1596.42, 3.718 → 1706.47, 4.912 → 1737.39, 3.396 → 1710.61,
  3.107 → 1707.96 (la última cuota del cuadro absorbe el residuo: 1707.97).
- En el caso 7 el suelo de 3.000 muerde en 4 de las 7 revisiones (0.228, 0.019, 0.013 y
  2.717 → 3.000) y no muerde en 3.328, 4.522 y 3.006. En el caso 8 las 7 revisiones
  cambian el TIN por el redondeo a 1/8: 0.618→0.625, 0.409→0.375, 0.403→0.375,
  3.718→3.750, 4.912→4.875, 3.396→3.375, 3.107→3.125.
- En los casos 9 y 10 el interés total se compara con el caso 1 (el mismo préstamo sin
  amortizar: 65743.56). El ahorro es diferencia de intereses, nunca una recomendación
  (P-08): decidir entre reducir cuota o plazo es del usuario.
- En los casos 9 y 10 la suma de amortizaciones del cuadro es 140000.00; junto con la
  amortización anticipada de 10000.00 suma el capital original de 150000.00. El JSON lo
  explicita (`sum_principal_repaid_schedule_only`).

### 3.1. De dónde salen los índices de los casos 6 a 8

Los tres casos comparten la misma forma: préstamo de `150000.00` desde `2019-01-01`
(cobro el día 1), 96 meses, tramo inicial de 12 meses y revisiones anuales con un
**desfase de referencia de 2 meses** (`reference_offset_months: 2`), como fija la
escritura. Por eso cada revisión usa el **Euríbor de noviembre del año anterior**: las
siete revisiones (2020-01 a 2026-01) usan los valores de noviembre de 2019 a 2025.

Los siete valores son **publicados** por el Banco de España (serie `D_1NBAF472`,
Euríbor a 1 año, captura local `testdata/bde/listaSeries-euribor-1y-MAX.json` del
2026-10-28): -0.272, -0.481, -0.487, 2.828, 4.022, 2.506 y 2.217. El generador verifica
cada mes y cada valor contra esa captura **antes** de calcular nada: si un valor o un mes
no coincide con lo publicado, aborta sin escribir ningún caso. Así la procedencia es
auditable: un valor real que no esté en la captura del Banco invalida la generación.

## 4. Cómo se generaron y cómo se contrastan

Cada JSON de `testdata/finance/` se escribió ejecutando el generador, nunca a mano:

1. **Método principal:** simulación exacta con `decimal.Decimal` (precisión 50) y
   `ROUND_HALF_UP` a céntimos, implementando las convenciones de la sección 1.
2. **Contraste:** la cuota de cada tramo se compara con `numpy_financial.pmt`. Si los dos
   métodos difieren en más de `0.01`, el generador **falla y no escribe ningún caso**.
   En `reduce-term` se comprueba además que el plazo resultante es el mínimo que cierra
   el préstamo con la cuota mantenida.
3. **Procedencia de los índices:** en los casos 6 a 8, cada mes y valor de índice se
   verifica contra la captura del Banco de España (`testdata/bde/`, serie `D_1NBAF472`);
   ver §3.1.
4. **Determinismo:** ejecutar el generador dos veces produce archivos idénticos byte a
   byte (fecha de generación fija `2026-10-28`, sin timestamps volátiles).

Cada JSON lleva en `source`: versiones de Python/NumPy/numpy-financial, métodos usados,
tolerancia del contraste, sha256 del propio generador y fecha de generación.

## 5. Qué NO soporta v1 y qué viene después

- Conversión de tipo efectivo anual a mensual: no (ver 1.1).
- Otras reglas de redondeo de cuota distintas de half-up a céntimos: no.
- Otras reglas de redondeo de tipo distintas de las listadas en `rate_rounding`
  (`none`, `1/8`, `1/4`, `0.05`): no.
- Base de cálculo por días (interest only pro-rata die): no; todo es mensualidad completa.

Los casos reales de H-03 (recibos bancarios anonimizados) **reemplazarán o confirmarán**
estos casos cuando estén disponibles: este spike es el sustituto acordado con el usuario,
no su reemplazo definitivo. Si un caso de referencia discrepa del cálculo de
`internal/finance`, se para y se pregunta; nunca se edita el caso para que pase
(arranque §9).
