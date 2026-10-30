# Casos de referencia de finanzas

Casos de referencia para probar `internal/finance` (FIN-01 a FIN-04) contra valores
calculados **fuera del código Go del proyecto**. Las convenciones de cálculo que estos
casos fijan están en `docs/calculos.md`; léelo antes de tocar nada de aquí.

## Qué hay

- `case-01` a `case-05`: préstamos de tipo fijo (caso base, tipo 0 %, tipo bajo, plazo
  corto, importes "feos").
- `case-06` a `case-08`: revisión (mixta con revisiones anuales, suelo, redondeo de
  escritura 1/8). Los valores de índice son **reales y publicados** del Euríbor a 1 año
  (Banco de España, serie `D_1NBAF472`, meses de noviembre de 2019 a 2025); cada JSON lo
  marca (`index_values_status: "published"`, con mes, valor y serie por entrada).
- `case-09` y `case-10`: amortización anticipada en los dos modos
  (`reduce-payment` y `reduce-term`), comisión 0 %.
- `case-11`: contraste contra un **recibo bancario real** de una hipoteca mixta
  (período 2021-06, anonimizado). No lo genera `generate.py`: sus valores esperados están
transcritos del papel, y lo que prueba es que nuestras convenciones producen los mismos
números que el banco (TIN desde índice y diferencial, e interés del mes redondeado a
céntimos). El recibo no declara capital inicial ni plazo original, así que el caso cubre la
cadena tipo → interés de un mes, no un cuadro completo. Es también el ejemplo que fija la
convención del mes de referencia: revisión el 2021-05-01 con el índice de **marzo**, porque
el BOE aún no había publicado abril.

La procedencia del índice de los casos 6 a 8 se comprueba contra
`testdata/bde/listaSeries-euribor-1y-MAX.json`: el generador verifica cada mes y cada
valor contra esa captura del Banco de España antes de calcular nada, y aborta sin
escribir ningún archivo si algo no coincide.

Cada JSON lleva `inputs` (todo lo necesario para recalcular a mano), `expected` (cuota,
totales, primera y última fila del cuadro, suma de amortizaciones y capital final) y
`source` (versiones de Python/NumPy/numpy-financial, métodos usados, sha256 del
generador, fecha de generación).

## Cómo se generaron

Con `generate.py`: simulación exacta con `decimal.Decimal` y `ROUND_HALF_UP` a céntimos,
contrastando la cuota de cada tramo con `numpy_financial.pmt`. Si los dos métodos
difieren en más de 0.01, el generador falla y no escribe ningún caso. Ejecutarlo dos
veces produce archivos idénticos byte a byte.

Los números de estos archivos **no se editan a mano**; se regeneran:

```bash
python testdata/finance/generate.py
```

Requisitos: Python 3 con `numpy` y `numpy-financial`
(`pip install numpy numpy-financial`; el resto es biblioteca estándar).

Versiones usadas para generar los casos actuales: Python 3.14.5, NumPy 2.5.4,
numpy-financial 1.1.0. La fecha de generación fijada es 2026-10-28. El sha256 del
generador que produjo estos archivos está dentro de cada JSON (`source.generator_sha256`).

## Regla dura (arranque §9)

**Ningún agente edita un caso de referencia para que pase un test.** Si
`internal/finance` discrepa de un caso, se para y se pregunta al usuario. El caso solo
cambia regenerándolo con `generate.py` tras una decisión explícita y documentada, o
cuando los casos reales de H-03 (recibos bancarios) los reemplacen o confirmen.
