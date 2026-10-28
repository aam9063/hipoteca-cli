# Review gate · SPIKE-02: fórmulas, convenciones y casos de referencia

## Cambio bajo revisión

- **Tarea:** SPIKE-02 — `odd/tasks/SPIKE-02-calculos.md`.
- **Contenido:** `docs/calculos.md`, `testdata/finance/generate.py`, diez JSON de casos y
  `testdata/finance/README.md`.
- **Riesgo declarado:** medio-alto. No hay código de producto, pero estos números son el
  patrón contra el que FIN-01 a FIN-04 validarán todo el dinero del proyecto.
- **Revisión:** el autor de la tarea **no** validó con su propio generador. Se escribió una
  **tercera implementación** independiente (Decimal, en Python, sin leer `generate.py`) y se
  comparó contra los fixtures. No corrió review nativo (`nativeRisk: unassessable`,
  `package-local-binary-missing`) ni verificador independiente (sin ítem de alto riesgo
  nombrado).
- **Decisión del usuario (2026-10-28):** aprobadas las convenciones, incluida la 1 (tipo
  mensual nominal) y la 4 (interés redondeado antes de amortizar); pedido explícito de
  regenerar los casos 6-8 con **índices reales ya publicados**; autorizado arrancar FIN-01
  con estos casos.

## Segunda ronda: casos 6-8 con datos reales

Los tres casos de revisión ya no usan índices hipotéticos. Mismo préstamo para los tres:
`150000.00`, 96 meses desde 2019-01, tramo inicial de 12 meses y revisiones anuales con
`reference_offset_months: 2`, es decir **siete revisiones** cuyo mes de referencia es el
noviembre del año anterior. Los siete valores son reales y ya publicados:

| Mes de referencia | Euríbor 1 año |
|---|---|
| 2019-11 | -0.272 |
| 2020-11 | -0.481 |
| 2021-11 | -0.487 |
| 2022-11 | 2.828 |
| 2023-11 | 4.022 |
| 2024-11 | 2.506 |
| 2025-11 | 2.217 |

Cada JSON lleva la procedencia (`{revision, reference_month, value, status: "published",
series}`) y un bloque `index_fixture` con la ruta, la serie y el `sha256` de la captura del
Banco (`a8dfd6f5…54362b5`). **El generador verifica cada valor contra el fixture y aborta
si no coincide**, así que la trazabilidad no es una promesa: es un paso del programa.

Verificación con la tercera implementación (la misma independiente de la primera ronda),
recalculada para 96 meses y siete tramos:

| Caso | Cuotas de revisión | Interés total mío | Fixture | Cierre |
|---|---|---|---|---|
| 6 mixta | las 7 coinciden | 9660.61 | 9660.61 | 0.00 |
| 7 suelo | las 7 coinciden | 20702.07 | 20702.07 | 0.00 |
| 8 redondeo 1/8 | las 7 coinciden | 12315.87 | 12315.87 | 0.00 |

Cuotas iniciales: 1642.74 / 1793.71 / 1766.26, idénticas. El suelo muerde en 4 de las 7
revisiones del caso 7 (0.228, 0.019, 0.013 y 2.717 → 3.000) y no en 3.328, 4.522 y 3.006;
las 7 revisiones del caso 8 cambian de tipo por el redondeo a 1/8. Determinismo confirmado:
`sha256` agregado de los diez JSON idéntico antes y después de re-ejecutar
(`3f9c78d4…dfd1847`).

Los casos 1-5, 9 y 10 cambiaron **una sola línea** cada uno: el `generator_sha256` embebido,
consecuencia mecánica de editar el generador. Dejar el hash viejo habría falsificado la
procedencia.

## Verificación ejecutada

### 1. Tercera implementación (la prueba principal)

Implementación propia y separada, escrita sin mirar el generador: `Decimal` con precisión
60, redondeo half-up a céntimos, fórmula de anualidad, última cuota absorbiendo el residuo.
Resultado contra los fixtures, campo por campo:

| Caso | Cuota | Interés total | Total pagado | Capital final | 1.er interés | 1.ª amortización | Última cuota |
|---|---|---|---|---|---|---|---|
| 1 | ✅ 719.14 | ✅ 65743.56 | ✅ 215743.56 | ✅ 0.00 | ✅ 387.50 | ✅ 331.64 | ✅ 720.70 |
| 2 | ✅ 500.00 | ✅ 0.00 | ✅ 120000.00 | ✅ 0.00 | ✅ 0.00 | ✅ 500.00 | ✅ 500.00 |
| 3 | ✅ 519.09 | ✅ 3435.95 | ✅ 93435.95 | ✅ 0.00 | ✅ 37.50 | ✅ 481.59 | ✅ 518.84 |

Los casos 4 y 5 se compararon con el mismo script: cuota, interés total, total pagado,
capital final, primera y última fila, **sin discrepancias**.

### 2. Casos 6 a 10: puntos de control

No se compararon las filas intermedias una por una; sí todos los puntos donde un error se
propagaría:

| Comprobación | Mío | Fixture |
|---|---|---|
| Saldo tras 60 cuotas (3.10 %, 300 m) | 128506.54 | 128506.54 |
| Interés / amortización del mes 60 | 332.97 / 386.17 | 332.97 / 386.17 |
| `reduce-payment`: cuota sobre 118506.54 a 240 m | 663.18 | 663.18 |
| `reduce-term`: meses necesarios con cuota 719.14 | 215 → 25 ahorrados | 275 m, 25 ahorrados |
| Caso 7: cuota inicial 3.500 % / tras suelo 3.000 % | 869.94 / 833.54 | 869.94 / 833.54 |
| Caso 8: redondeo 1/8 de 3.101 | 3.125 | 3.125 |
| Caso 8: cuota de revisión a 3.125 %, 228 m | 841.23 | 841.23 |
| Caso 6: cuota inicial 1.25 % / revisión 1 a 2.390 % | 706.70 / 782.90 | 706.70 / 782.90 |

Coherencia de cadenas comprobada: saldo del fixture en el mes siguiente = mi saldo menos la
amortización publicada (143356.63 − 497.38 = 142859.25; 144499.78 − 464.93 = 144034.85;
118506.54 − 357.04 = 118149.50; 118506.54 − 413.00 = 118093.54).

### 3. Determinismo

`sha256` del conjunto de los diez JSON antes y después de re-ejecutar el generador:
idénticos (`26a1289c4c4ecfb194c52033ad2319c178d42cfc889629e5e17282a987b614a5`). Sin
timestamps volátiles.

### 4. El cross-check falla de verdad

No basta con que el código diga que falla. Sobre una copia del directorio, con un módulo
`numpy_financial` falso que devuelve `1234.56`:

```
ERROR: cuota (TIN 3.10 %, 300 meses): cuota Decimal=719.14328... vs numpy_financial.pmt=1234.56
Los dos métodos independientes discrepan: NO se escribe ningún caso.
EXIT=2
```

Y los `sha256` de los JSON de la copia quedaron **intactos**: el fallo no escribe nada.

### 5. Documento

- El caso 1 resuelto a mano es **reproducible y correcto**: interés del mes 1 `387.50`,
  amortización `331.64`, saldo `149668.36`, última cuota `720.70 = 718.84 + 1.86`,
  intereses `65743.56`. Todos coinciden con mi cálculo.
- Las seis convenciones del documento son las del documento de tarea, con la justificación
  añadida. El orden crítico (suelo **antes** del redondeo de escritura) está explícito.
- `bash scripts/check.sh` sigue en verde tras añadir los archivos.

## Hallazgos

| Sev. | Dónde | Qué | Resolución |
|---|---|---|---|
| nit | `testdata/finance/*.json` | Los campos se llaman `principal_repaid`/`outstanding_after`/`final_outstanding`, no los nombres que asumí al escribir el comparador | Sin defecto: mi comparador se adaptó. Se anota para que FIN-01 use esos nombres |
| nit | `docs/calculos.md` §3 | La columna "cuota tras el cambio" del caso 6 muestra el **tramo 8**, no el 1 | Aceptable y con nota al pie que lo aclara; los ocho tramos están en el JSON |
| observación | fixtures | Verificados completos los casos 1-5 y por puntos de control los 6-10 | Un error en una fila intermedia de 9 o 10 sólo podría venir de la simulación, que es la misma función ya validada en 1-5 |

No se encontraron defectos de cálculo. Los dos "nit" no afectan a ningún número.

## Veredicto

**PASS.** Las convenciones fueron aprobadas por el usuario, los casos 6-8 se regeneraron
con índices reales y verificados contra el fixture del Banco, y la tercera implementación
coincide en los diez casos: cinco campo por campo y cinco por puntos de control y tramos.

Lo que este gate **no** puede afirmar: que los casos coincidan con lo que cobra un banco
real. Eso es H-03 y sigue abierto. Dos implementaciones independientes que coinciden
demuestran que la aritmética es correcta según las convenciones; no demuestran que las
convenciones sean las de todas las escrituras.

## Pendiente para el usuario

1. H-03 sigue abierto: los recibos reales confirmarán o reemplazarán estos casos. Es la
   única prueba que puede detectar que una convención de la escritura no es la nuestra.
