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
- **Cierre pendiente:** la aprobación del usuario sobre la tabla de casos (SPIKE-02.5) es un
  criterio de aceptación de la tarea, no una formalidad.

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

**PASS CON FIXES MENORES** → en la práctica: **aprobable**, sujeto a la revisión del
usuario (SPIKE-02.5), que es criterio de aceptación de la tarea.

Lo que este gate **no** puede afirmar: que los casos coincidan con lo que cobra un banco
real. Eso es H-03 y sigue abierto. Dos implementaciones independientes que coinciden
demuestran que la aritmética es correcta según las convenciones; no demuestran que las
convenciones sean las de todas las escrituras.

## Pendiente para el usuario

1. Aprobar la tabla de los diez casos (`docs/calculos.md` §3) antes del merge.
2. Decidir si los índices hipotéticos de los casos 6-8 (marcados
   `index_values_status: assumed-hypothetical`) sirven así o se prefieren valores reales
   ya publicados.
3. H-03 sigue abierto: los recibos reales confirmarán o reemplazarán estos casos.
