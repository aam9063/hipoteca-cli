# Review gate · FIN-03: amortización anticipada con comisión

## Cambio bajo revisión

- **Tarea:** FIN-03 — `odd/tasks/FIN-03-amortizacion.md`.
- **Rama:** `hc/fin-03-amortizacion` (base `dev`).
- **Riesgo declarado:** ítem 6.
- **Verificador independiente:** sí (`independentVerifier: true`).

## Veredicto del verificador: PASS (0 blockers, 0 majors)

Lo que confirmó con sondas propias, no leyendo código:

- **La comisión no toca la economía**: con el mismo importe y comisiones 0 / 0,5 / 3 / 2,005 %,
  el pago, el plazo, el ahorro y **todas las filas** del cuadro son idénticas; sólo cambian
  `Fee` y `Outlay`. Y `suma de amortizaciones + importe = capital`, con comisión incluida.
- **Casos 9 y 10 al céntimo**, leídos de los JSON: `663.18` con `3431.18` ahorrados; `275`
  meses, `25` ahorrados, `8004.38`, cierre exacto `0.00`.
- **Cierre de `reduce-term`** en las tres formas: residuo exacto al céntimo, residuo a un
  céntimo de distancia, y plazo mínimo comprobado por el saldo de la penúltima fila.
- **Propiedad condicional** verificada en 850 combinaciones propias (más amplias que la
  rejilla del autor): tipo ≥ 0 ⇒ ahorro ≥ 0; tipo < 0 ⇒ ahorro ≤ 0 y acotado por el
  subsidio de la vida intacta.
- **Sentinels**: los ocho casos devuelven su centinela con resultado cero, sin filtrar un
  `Prepayment` parcial.
- **Integridad del núcleo**: `git diff dev...HEAD` sobre `finance.go`, `schedule.go`,
  `rate.go` y `compound.go` está **vacío**; `testdata/**` también. Sin float, sin red, sin
  reloj ni E/S en `prepay.go`.

## Los dos hallazgos y qué se hizo

**M2 — mi test era tautológico.** La «identidad exacta del subsidio no devengado»
(`InterestSaved = IBW − AfterTail + BeyondWindowTail`) se reduce algebraicamente a
`baseTail − AfterTail`: **la ventana se cancela**. El verificador lo demostró y además
comprobó el campo contra un `Amortization` fresco en 850 combinaciones, donde coincidió. El
código estaba bien; **el test no probaba nada**.

Corregido: la identidad se retiró y se reemplazó por la comparación contra un baseline
calculado **fuera** de `Prepay`. Y se demostró que el test nuevo **sí puede fallar**:
corrompiendo el campo `+1.00` los dos tests de ventana fallan, restaurando vuelven a verde.
Eso es exactamente lo que la tautología no podía hacer.

**M1 — amortización que agota el saldo.** El cuadro cierra **por el evento**: la última fila
es la del mes del evento y lleva el capital anterior, `BalanceAfter` queda en `0.00` exacto,
`TermAfter` es el mes del evento, y `Schedule.TotalPaid` no incluye el desembolso. Es
coherente con la convención de los casos de referencia, pero choca con la redacción «el
cuadro cierra a 0.00» del checklist. Documentado en `prepay.go`, en `docs/calculos.md` §1.5 y
en el Log como **obligación de CMD-02**: el comando debe presentar el evento como una línea
propia, o el usuario verá un cuadro que no cierra.

## Verificación del autor

- `bash scripts/check.sh` → **EXIT=0**, «No vulnerabilities found».
- Los cinco `TestPrepay*` y los cuatro tests de regresión del verificador → PASS, 2376
  combinaciones de rejilla sin fallos.
- `git diff internal/finance/prepay.go` filtrado a líneas de código: **vacío** (sólo
  comentarios), confirmando que la mutación de prueba se revirtió.

## Límites declarados

- Amortización con importe 0 es un no-op válido; sólo el negativo es error.
- H-03 (recibos reales) sigue abierto.

## Veredicto

**PASS.** La comisión no contamina el capital, el cierre de plazo está probado en sus tres
formas y la propiedad del ahorro es condicional al signo del tipo, con la razón escrita.
