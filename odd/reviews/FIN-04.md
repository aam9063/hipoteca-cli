# Review gate · FIN-04: comparativa de índices

## Cambio bajo revisión

- **Tarea:** `odd/tasks/FIN-04-compare.md`.
- **Rama:** `hc/fin-04-compare` (base `dev`).
- **Riesgo declarado:** ítem 6 — dinero sobre históricos reales, en un comando que puede usarse
  para sostener una reclamación.
- **Verificador independiente:** sí, contra la **API en vivo** con su propio cliente HTTP, más
  recálculo completo en aritmética racional exacta.

## Veredicto del verificador: PASS WITH FIXES (2 majors, 4 minors, 1 nit)

Lo que confirmó contra el Banco en vivo:

- **Cada revisión usa el valor de su propio mes de referencia**: Euríbor 2022-04 → 0.013,
  2023-04 → 3.757, 2024-04 → 3.703, 2025-04 → 2.143, 2026-04 → 2.747; IRPH 1.54 / 3.683 /
  3.823 / 2.851 / 2.882. Cada valor coincide con el de la API para **ese** mes, y cambia año a
  año — un valor reutilizado se detectaría.
- **El corte de cobertura no tiene off-by-one**: moverlo un mes cambia el total en 2.288 €
  (160.585,32 en el mes 71), así que la sensibilidad está probada. Un préstamo que termina
  justo en el corte no lleva aviso; uno que se extiende, sí.
- **Totales al céntimo** contra su propio recálculo racional: 162.873,90 / 12.873,90 y
  165.410,29 / 15.410,29.
- **P-08**: bloques simétricos, sin orden ni nombre que insinúe veredicto.

## Los dos majors: un cambio de tipo fabricado

El verificador encontró que `revision` sobre un préstamo **fijo** reportaba `raw/new = 4.647 %`
y una cuota de `905,33 €` frente a `599,85 €`, con `warnings: []` y el índice `published`; y que
`compare` sobre un fijo armaba un historial de revisiones inexistente marcado `published` y
`assumed: false`.

**En un comando que puede sostener una reclamación, presentar un dato inventado es peor que
negarse a responder.** Y el error de alcance fue mío: tomé el advisory de LOAN-01 («un fijo sin
`sync` no puede revisarse aunque su cuadro sea exacto») como un problema de **capital** y pedí
levantar sólo eso, sin caer en que un préstamo fijo **no tiene revisiones de tipo**.

Corregido:

- `revision` sobre un fijo → `usage`/2 con el hecho y un hint a `schedule`/`amortize`; la
  comprobación va **antes** de cualquier acceso a la fuente o cálculo.
- `compare` sobre un fijo sigue existiendo —«¿qué habría pagado si fuera variable?» es legítimo—
  pero **todo el bloque propio va `assumed: true`** con un aviso P-06 que dice que es una
  recomposición hipotética, no su historia.
- Un fijo sin revisiones dentro del plazo → `usage`/2 explícito: no hay nada que comparar.

## Menores corregidos

| Hallazgo | Corrección |
|---|---|
| El mensaje de «cero comparables» no nombraba el mes | Ahora dice `2027-04`, para distinguir «esperá la publicación» de «acá hay un hueco»; un hueco real sigue dando `index_not_published`/4 |
| La convención del signo de la diferencia no estaba escrita | `compare --help`: la diferencia es **alternativa − propia**, positiva significa que con el alternativo se habría pagado más |
| El help de `--to` decía que acota las revisiones mostradas | Ahora dice que acota también el cálculo y los totales, que corren hasta la revisión siguiente |
| Un fijo debía declarar `initial_period_months` | Se mantiene: es el dato que define cuándo empieza la primera revisión; documentado como límite |

Y un arreglo colateral que el propio test nuevo destapó: un `serieDe` ordenaba meses con el
orden de un map, lo que hacía el corte de cobertura **flaky**. Corregido con ordenación y
verificado con `-count=5`.

## Verificación del autor

- `bash scripts/check.sh` → **EXIT=0**.
- Binario: `compare` sin ventana → cobertura 2022-04 a 2026-04, Euríbor `0.013` / IRPH `1.540`,
  totales propio 161.790,95 vs alternativo 160.942,42 (diferencia −848,53), aviso de cobertura y
  cero palabras de recomendación.
- `revision` sobre fija → `usage`/2 con el tipo del contrato. `compare` sobre fija con
  revisiones → bloque propio entero `assumed: true` con aviso hipotético. Sin revisiones en el
  plazo → `usage`/2.

## Límites declarados

- `--from` es **sólo de presentación**: los totales cubren desde el inicio del préstamo, porque
  cortar el arranque exigiría un capital intermedio que no se puede conocer. Está en el help.
- Un préstamo fijo con `initial_period_months` igual al plazo no tiene revisiones y por tanto no
  se puede comparar; la hipótesis requiere declarar un periodo inicial.
- H-03 (recibos reales) sigue abierto.

## Veredicto

**PASS tras la ronda única de correcciones.** La comparación usa los valores reales de cada mes,
declara hasta dónde cubre, no proyecta el futuro y ya no presenta como historia lo que es una
hipótesis.
