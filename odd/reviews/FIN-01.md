# Review gate · FIN-01: cuota, cuadro de amortización y tipo 0 %

## Cambio bajo revisión

- **Tarea:** FIN-01 — `odd/tasks/FIN-01-cuota-cuadro.md`.
- **Candidato:** commit `541ad6f` en `hc/fin-01-cuota` (base `dev`).
- **Contenido:** `internal/finance/{finance,schedule}.go`, sus tests, la guarda anti-float
  y la dependencia `shopspring/decimal`.
- **Riesgo declarado:** alto. Es el núcleo de dinero del proyecto.

## Verificación del autor

### 1. Revisión del código

- `roundHalfUpCents` calcula `floor(x·100 + 1/2)` sobre racionales y devuelve el número
  entero de céntimos. Con la validación vigente el tipo no puede ser negativo, así que el
  redondeo es half-up en todo el dominio aceptado; la precondición **está** impuesta por
  `validate`, no supuesta.
- `powRat` hace exponenciación binaria con `big.Rat`: `(1+i)^n` es exacto.
- La cuota exacta se convierte a céntimos **antes** de pasar a `decimal`, con
  `decimal.NewFromBigInt(cents, -2)`. Aquí estuvo el bug real: `big.Rat` normaliza
  `71914/100` a `35957/50` y usar `SetFrac().Num()` devolvía 35957, es decir una cuota de
  359,57 € en vez de 719,14 €. El caso de referencia lo cazó; la corrección es usar el
  entero de céntimos directamente.
- La última cuota es `balance + interest` y no se emite ningún residual intermedio.

### 2. Contraste fuera de los fixtures

"Pasa los cinco casos" puede significar sólo que está ajustado a cinco casos. Prueba con un
préstamo que no está en `testdata/finance` (250000.00 al 4.15 %, 372 meses), contra una
implementación independiente en Python:

| Magnitud | Independiente | `internal/finance` |
|---|---|---|
| Cuota | 1195.58 | 1195.58 |
| Interés total | 194754.99 | 194754.99 |
| Interés / amortización mes 1 | 864.58 / 331.00 | idénticos |
| Saldo tras mes 1 | 249669.00 | 249669.00 |
| Última cuota | 1194.81 | 1194.81 |
| Cierre | 0.00 | 0.00 |

El test temporal se borró; el árbol quedó limpio.

### 3. Tests y guardas (leídos, no sólo ejecutados)

- Los cinco casos fijos se leen de `../../testdata/finance/*.json`: no hay ni un número
  esperado escrito a mano en el test.
- Las comparaciones usan `decimal.Equal`, no formateo de cadenas (evita falsos verdes por
  "719.140" vs "719.14").
- Las propiedades comprueban suma de amortizaciones = principal, saldo no creciente,
  `TotalPaid = TotalInterest + Principal`.
- La guarda `go/ast` falla si aparece `float64`/`float32` en los ficheros no-test.

### 4. Verificación de arranque

`bash scripts/check.sh` → **EXIT=0** (gofmt, tidy sin diff, vet, staticcheck, govulncheck,
tests).

## Verificador independiente

Escalado el ítem de riesgo 6 del dimensionado de tareas: los cinco casos cubren tipos de 0 a
6.25 % y plazos de 60 a 347 meses, de modo que una regresión de redondeo o de residual en
los extremos no la atraparía ningún test actual. Motivo aceptado
(`independentVerifier: true`) y ejecutado por un subagente de contexto limpio y sólo
lectura, con instrucciones de intentar romper el half-up, el residual en casos extremos y
la guarda anti-float.

**Veredicto del verificador: PASS WITH FIXES**, con un hallazgo sustantivo.

### Hallazgo del verificador (aceptado)

**MAJOR — el residual no impide saldos negativos.** Reproducido: `150000.00` al
**23.62 %** a **480 meses**. El mes 479 paga `2952.76` sobre un saldo que sólo admite
`2743.77`, deja el saldo en `−264.99` y el mes 480 emite un **pago negativo** (`−270.20`,
interés `−5.21`). La suma de amortizaciones da `149735.01`, no `150000.00`. La invariante
que declara FIN-01.4 («el capital nunca es negativo») es falsa para entradas que la API
acepta.

Causa raíz: la cuota se redondea half-up a céntimos, y ese excedente de hasta medio céntimo
por mes, acumulado durante cientos de meses, **liquida el préstamo antes del mes `Months`**;
el bucle seguía emitiendo filas de meses que ya no existen. No es alcanzable a tipos
hipotecarios españoles (una rejilla fina con tipos ≤ 7 % pasa entera), pero es alcanzable.

Corrección aplicada: cuando la cuota excede `saldo + interés` en cualquier mes, ese mes pasa
a ser el último (`pago = saldo + interés`, saldo a `0`) y el cuadro termina ahí, de modo que
`len(Rows) ≤ Months`. Los cinco casos de referencia siguen intactos.

### Otros hallazgos, y qué se hizo con ellos

| Sev. | Hallazgo | Decisión |
|---|---|---|
| minor | `roundHalfUpCents` documenta `floor(x·100 + 1/2)`; con entradas negativas `Quo` trunca hacia cero. No es alcanzable salvo por el fallo anterior, y `validate` impide tipos negativos | Se documenta la precondición; la corrección del MAJOR elimina el camino negativo |
| — | El núcleo aritmético está limpio: `Decimal.Rat()` exacto, exponenciación binaria exacta, una sola conversión y un solo redondeo, sin trampa de normalización de `big.Rat` | Sin acción |
| — | Los tests leen los valores de los JSON, comparan con `decimal.Equal`, y la lógica del residual es **load-bearing**: quitarla hace fallar los casos 1, 3, 4 y 5 (el 2 no tiene residual porque `120000/240` es exacto) | Confirmado, sin acción |
| — | **La guarda anti-float es superficial**: el verificador coló `decimal.NewFromFloat`, `strconv.ParseFloat` y `reflect...Float()` en el paquete y el test siguió en verde | Se acepta y se documenta el límite: detecta tipos `float64`/`float32` en este paquete, no un float escondido tras otra API, `interface{}` o `unsafe`. El linter completo es OPS-02 |
| — | La API es extensible: una revisión es `Payment(saldo, TIN nuevo, meses restantes)` con suelo y redondeo aplicados por el llamador; FIN-02 no necesita rediseño | Sin acción |

## Corrección y re-verificación

El fallo se corrigió con cierre anticipado: en el mes en que la amortización programada
alcanza o supera el saldo, esa mensualidad pasa a ser la última (`pago = saldo + interés`,
saldo a `0`) y el bucle **se detiene**. `len(Rows) ≤ Months`; un préstamo normal conserva
exactamente `Months` filas y el residual de la convención 1.3 no cambia.

Dos cosas que conviene registrar del propio proceso de corrección:

1. El primer intento cerró el préstamo pero **no** cortó el bucle, y dejó filas fantasma con
   saldo `0.00`. Lo detectó la aserción nueva «saldo cero sólo en la última fila», no la
   lectura del código. La rejilla de propiedades hizo su trabajo antes de que yo viera el
   diff.
2. `TestScheduleGridProperties` recorre 245 combinaciones (5 capitales × 7 tipos de
   0.01 % a 25 % × 7 plazos de 1 a 480 meses) y falla en la combinación exacta del
   hallazgo, que es lo que la hace útil: no es una lista de casos felices.

Verificación del autor sobre la corrección, con implementación independiente:

| Magnitud | Independiente | `internal/finance` |
|---|---|---|
| Cuota | 2952.76 | 2952.76 |
| Filas del cuadro | 479 | 479 |
| Última cuota | 2687.77 | 2687.77 |
| Interés total | 1264107.05 | 1264107.05 |
| Cierre | 0.00 | 0.00 |

Los cinco casos de referencia siguen coincidiendo al céntimo, sin cambios en sus
aserciones ni en `testdata/finance/**`. `bash scripts/check.sh` → **EXIT=0**.

## Veredicto

**PASS.** El defecto que encontró el verificador está corregido y re-verificado; el resto de
su informe no pedía cambios: núcleo aritmético limpio, tests que muerden (quitar el residual
rompe cuatro de cinco casos), procedencia intacta y API extensible para FIN-02.

Límites que quedan escritos a propósito:

- La guarda anti-float detecta `float64`/`float32` **en este paquete**, y el verificador
  demostró que no ve un float escondido tras otra API, `interface{}` o `unsafe`. El linter
  completo es OPS-02.
- Los cinco casos de referencia no cubrían el régimen extremo; ahora sí hay rejilla, pero la
  validación contra la realidad sigue siendo H-03.

## Pendiente para el usuario

1. H-03 (recibos reales) sigue abierto y es lo único que puede validar las convenciones
   contra la realidad.
2. El límite de la guarda anti-float: un test de AST no detecta un `float64` escondido en
   un `interface{}`, en una conversión o vía `unsafe`. El linter completo es OPS-02.
