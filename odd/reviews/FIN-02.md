# Review gate · FIN-02: revisión, suelo, redondeo de escritura y mixta

## Cambio bajo revisión

- **Tarea:** FIN-02 — `odd/tasks/FIN-02-revision.md`.
- **Rama:** `hc/fin-02-revision` (base `dev`).
- **Riesgo declarado:** alto, ítem 6.
- **Verificador independiente:** sí (`independentVerifier: true`).

## Veredicto del verificador: PASS (0 blockers, 0 majors)

Lo que probó y confirmó, con sondas propias:

- **El orden importa y está bien**: `3.06` con suelo `3.063` y redondeo `1/8` da `3.125`; con el
  orden invertido saldría `3.063`, que ni siquiera es múltiplo de 1/8. `nil` de suelo → TIN
  negativo (`-0.481 + 0.2 = -0.281`), no cero.
- **Redondeo simétrico**: puntos medios exactos en los cuatro modos y en ambos signos, más
  valores a ±0.0000001 del punto medio. Para no negativos, idéntico al anterior: los diez
  casos de referencia con `sha256` iguales a `dev` y `git diff -- testdata/` vacío.
- **Tipos negativos de punta a punta**: −0,3 % a 360 meses y **−5 % a 300 meses** cierran
  exactos en `0.00`, con la suma de amortizaciones igual al capital.
- **`CompoundSchedule`**: la cuota de cada tramo se recalcula sobre **los meses que quedan al
  término** (fila 13: `Payment(pendiente, 3 %, 48) = 2176.54`; sobre el tramo habría sido
  `8328.22`), y el cuadro coincide **fila a fila** con el núcleo encadenado a mano en los
  cuatro tramos. Sin off-by-one en el borde: el interés del mes 13 se calcula sobre el capital
  del mes 12.
- **`ReferenceMonth`** bajo cuatro zonas horarias, incluido el 31 de enero: siempre UTC.

## Correcciones de esta ronda

| Qué | Por qué |
|---|---|
| **Tipos negativos soportados en el núcleo** | La decisión 3 permitía un TIN negativo pero `Payment`/`Amortization` lo rechazaban: `NewRate` devolvía −0,3 % y el cuadro no se podía construir. Es el caso de las hipotecas sin suelo con Euríbor negativo — la población que peleó las cláusulas suelo. Verificado contra implementación independiente: −0,30 % a 120 meses sobre 100000 → cuota 820,79, intereses −1.504,98, cierre 0,00. |
| **Redondeo half-up lejos de cero, simétrico** | `floor(x + 1/2)` redondeaba hacia +∞: `−0.0625` con octavos daba `0` en vez de `−0.125`. Para no negativos no cambia nada, y los diez casos lo prueban al céntimo. |
| **`ErrUnpayableSchedule`** (minor del verificador) | Con TIN −50 % a 480 meses la cuota redondea a `0.00` y el cuadro emitía 151 filas sin progreso. Ahora devuelve error. |
| **Mensaje del error corregido por el autor** | La primera versión decía «el préstamo no se puede amortizar nunca», y eso es **falso**: la última cuota absorbe el capital y el cuadro cerraba en `0.00`. Lo que no existe es un plan mensual válido. El mensaje ahora dice eso, y el comentario aclara el matiz. Un error que describe mal el hecho es peor que no tenerlo. |
| **Documentación al día** | `compound.go` afirmaba que los tipos negativos no estaban soportados; `docs/calculos.md` describía el half-up sin precisar el signo. Ambos corregidos. |

## Un diagnóstico ambiental que no era cierto

El verificador informó que `govulncheck` fallaba «también en `dev`» con 23 vulnerabilidades de
stdlib. Lo comprobé en este repositorio: **`exit 0` en la rama y en `dev`**, con
`go version go1.26.10` y `toolchain go1.26.10` en `go.mod`. Su copia scratch no aplicó el
toolchain pineado. Es el tercer diagnóstico ambiental equivocado de la sesión y la razón por
la que el padre contrasta: **un verificador independiente también se equivoca**, y su error se
propaga si nadie lo mira.

## Límites que quedan escritos

- Un capital de `0.01` con plazo mayor a un mes ahora devuelve `ErrUnpayableSchedule` en vez de
  un cuadro con filas de `0.00`. Es consecuencia de la misma regla; el caso de un mes sigue
  válido porque la última cuota cierra en la primera fila.
- El cierre anticipado de FIN-01 (479 de 480) no se reproduce con tramos al mismo tipo tras una
  recomputación: es otra cadena de redondeos, no un defecto.
- H-03 (recibos reales) sigue abierto: es lo único que valida estas convenciones contra un banco.

## Veredicto

**PASS.** El orden suelo→redondeo, la simetría del signo y el encadenado de tramos están
probados con casos que los distinguen, no sólo con los felices. Los diez casos de referencia
quedan intactos al céntimo.
