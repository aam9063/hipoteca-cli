# FIN-03: amortización anticipada en ambos modos con comisión

## Objetivo

Calcular qué pasa si el usuario amortiza una cantidad antes de tiempo: cuánto paga de
comisión, qué cuota le queda (reduciendo cuota) o cuántos meses se ahorra (reduciendo plazo),
y cuántos intereses deja de pagar en cada caso.

## Problema y motivo

Es la pregunta «¿me conviene reducir cuota o plazo?» del arranque §1. La CLI **no puede
recomendar** (P-08): devuelve las dos cifras y el usuario decide. Y la comisión no es un
detalle: cambia el resultado lo suficiente como para invertir la comparación.

## Alcance

- Dentro:
  - `Prepay(loan, repayment, mode)` para `reduce-payment` y `reduce-term`.
  - Comisión `= importe · fee_pct / 100`, redondeo half-up simétrico.
  - Ahorro de intereses frente a no amortizar, y meses ahorrados en `reduce-term`.
  - Validación contra los casos 9 y 10 de `testdata/finance/` (comisión 0 %), y casos
    propios con comisión verificados con cálculo independiente.
- Fuera:
  - `amortize` como comando (CMD-02), escenarios y comparativa de índices (FIN-04).

## Decisiones

1. **La comisión se paga aparte, no reduce el capital.** El desembolso del usuario es
   `importe + comisión` y el capital baja exactamente por `importe`. Confundirlos daría un
   cuadro que no cierra con el recibo del banco.
2. **`reduce-payment` conserva el plazo restante** y recalcula la cuota con el saldo nuevo.
3. **`reduce-term` conserva la cuota** y el plazo es el número de meses que la cierran; la
   última cuota absorbe el residuo, como en todo el proyecto.
4. **El ahorro se mide contra el mismo préstamo sin amortizar**, no contra otra cosa: es la
   única comparación que responde la pregunta del usuario.
5. **Propiedad que debe cumplirse siempre (regla condicional según el signo del TIN)**:
   con un TIN **no negativo**, amortizar **nunca** aumenta los intereses restantes, en
   ninguno de los dos modos, con o sin comisión dentro del rango razonable. Con un TIN
   **negativo** (FIN-02, decisión 3) el ahorro es **negativo por construcción**: el
   interés mensual es a favor del prestatario y al amortizar renuncia al subsidio que el
   saldo pendiente devengaba, de modo que `InterestSaved` se informa con signo (p. ej.
   −0.75) y nunca se recorta a cero. Si un caso viola la regla correspondiente a su
   signo, es un defecto del cálculo.
6. `internal/finance` sigue puro: sin red, sin E/S, sin reloj. El mes de la amortización es
   un índice de cuota.

## Superficie de edición (nuestra)

- `internal/finance/prepay.go`, `internal/finance/prepay_test.go`, `internal/finance/doc.go`
- `odd/tasks/FIN-03-amortizacion.md`, `odd/reviews/FIN-03.md`

## Riesgo (nuestra)

**Alto**, ítem 6: es dinero y el redondeo de la comisión y del cierre de plazo no lo detecta
ningún test que no sea un caso de referencia. Lleva verificador independiente.

## Checklist

- [ ] FIN-03.1 `Prepay` en ambos modos · Criterio: reproduce los casos 9 y 10 al céntimo
      (ahorro 3431.18 reduciendo cuota; 8004.38 y 25 meses reduciendo plazo).
- [ ] FIN-03.2 Comisión · Criterio: `importe · pct / 100` con redondeo half-up simétrico,
      sumada al desembolso y no al capital.
- [ ] FIN-03.3 Propiedad · Criterio: sobre una rejilla de importes, tipos, plazos y
      comisiones, el ahorro es siempre ≥ 0 y el cuadro cierra a 0.00.
- [ ] FIN-03.4 Errores · Criterio: importe mayor que el capital, mes fuera del cuadro,
      comisión negativa y modo desconocido devuelven error centinela.
- [ ] FIN-03.5 Verificador independiente · Criterio: informe en `odd/reviews/FIN-03.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Decisiones fijadas; implementación no iniciada.

## Siguiente paso

Test en rojo contra los casos 9-10, implementación, verificación.

## Log

### L1 · 2026-10-28 · decisión del usuario

Tras cerrar CMD-01, se continúa con FIN-03, la última pieza de cálculo antes de CMD-02.

### L2 · 2026-10-28 · corrección de alcance de la decisión 5

Durante la implementación, la rejilla de la propiedad (2376 combinaciones) detectó que
la decisión 5 tal como estaba escrita sólo se cumple con TIN no negativo: las 474
violaciones son todas del tipo −0.30 %, cero en 0 %, 0.50 %, 3.10 % y 6.25 %. Con TIN
negativo el interés es a favor del prestatario y amortizar reduce el subsidio que
cobraría, así que el ahorro firmado es negativo por construcción, no un defecto del
cálculo. Corrección del usuario: decisión 5 reescrita como regla condicional según el
signo del tipo; el cálculo no cambia, `InterestSaved` conserva el signo y se expone
`InterestBeforeWindow` (interés del cuadro intacto en la misma ventana) para que el
signo sea interpretable. Documentado también en docs/calculos.md §1.5.

### L3 · 2026-10-28 · revisión independiente: tautología retirada y obligación M1

El verificador independiente (PASS, sin bloqueos ni mayores) detectó que la «identidad
exacta del subsidio no devengado» era tautológica: `IBW − vida nueva + cola fuera de
ventana` se reduce algebraicamente a `baseTail − vida nueva` (la ventana se cancela), de
modo que nunca habría detectado una `InterestBeforeWindow` mal calculada. Retirada la
identidad; el valor del campo se contrasta ahora contra un cuadro base fresco
(`Amortization` calculado fuera de `Prepay`) en la rejilla y en un test propio, y el
verificador ya lo había validado aparte sobre 850 combinaciones. Se añaden además los
tests duraderos de la revisión (la comisión no toca la economía; residuos a céntimo del
reduce-term a 0 % con prueba de plazo mínimo; valor cero junto a cada error centinela).

**Obligación M1 para CMD-02** (importe que agota el saldo): el cuadro devuelto cierra por
el propio evento — la última fila lleva el capital anterior al evento, `BalanceAfter` es
0.00 exacto, `TermAfter` es el mes del evento y `Schedule.TotalPaid` no incluye el
desembolso. CMD-02 debe renderizar el evento como línea propia. Documentado en el doc
de `Prepayment`/`Prepay` y en docs/calculos.md §1.5.
