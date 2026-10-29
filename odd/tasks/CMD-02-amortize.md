# CMD-02: comandos `amortize` y `scenario`

## Objetivo

Exponer los dos cálculos que faltan en la CLI: qué pasa si amortizas antes de tiempo
(reduciendo cuota o plazo) y qué cuota tendrías con distintos valores del índice.

## Problema y motivo

`Prepay` y el núcleo ya calculan todo esto, pero no hay forma de pedírselo sin escribir Go. Y
`amortize` tiene una trampa documentada en FIN-03 (M1): cuando la amortización agota el saldo,
el cuadro cierra **por el evento**, así que el comando tiene que presentar el evento como una
línea propia o el usuario ve un cuadro que no cierra.

## Alcance

- Dentro:
  - `hipoteca amortize --amount --date --mode reduce-payment|reduce-term|compare` sobre el
    cálculo suelto (`--principal --rate --months`), porque los préstamos guardados son
    LOAN-01.
  - `hipoteca scenario --index-values 1.5,2,2.5,3` con `--spread`, `--principal`,
    `--rate`/`--months` o `--months` + cuota: cada valor produce cuota y coste total.
  - Salida JSON de CORE-01 con `model.Decimal`, `schema_version`, entradas y atribución de
    datos procesados.
- Fuera:
  - `compare` (comparativa de índices con históricos reales): depende de FIN-04, que no está
    hecho. **No se expone un comando que no puede cumplir.**
  - Préstamos guardados (LOAN-01).

## Decisiones

1. **`amortize --mode compare` devuelve las dos opciones con sus cifras**, nunca una
   recomendación (P-08). Ni «conviene», ni «mejor», ni flechas de ganador: dos bloques y que
   el usuario decida.
2. **El evento se presenta explícitamente** (obligación M1 de FIN-03): cuando el importe
   agota el saldo, la salida marca que el préstamo se cierra en ese mes y no inventa un
   cuadro que termine en `0,00` por una cuota inexistente.
3. **`--date` es el mes del evento** (índice de cuota desde el inicio del cálculo), no una
   fecha del calendario: sin préstamo guardado no hay calendario. Se documenta en el `--help`.
4. **`scenario` acepta valores de índice explícitos** y los devuelve tal cual en la salida,
   marcados como `assumed` cuando no vienen de la API (P-06): un escenario nunca se presenta
   como dato oficial.
5. **Todos los importes y tipos como `model.Decimal`**, con la guarda de AST ya extendida a
   `internal/app` y `internal/cli`.

## Superficie de edición (nuestra)

- `internal/app/{amortize,scenario}.go` y sus tests, `internal/app/doc.go`
- `internal/cli/{amortize,scenario}.go`, `internal/cli/root.go` y sus tests
- `odd/tasks/CMD-02-amortize.md`, `odd/reviews/CMD-02.md`

## Riesgo (nuestra)

**Medio-alto**, ítem 3: contrato de salida nuevo sobre dinero ya validado.

## Checklist

- [ ] CMD-02.1 `amortize` en los tres modos con salida JSON · Criterio: reproduce los casos 9
      y 10 (663.18 / 3431.18 y 275 m / 8004.38) leídos de los JSON.
- [ ] CMD-02.2 `compare` devuelve ambas opciones · Criterio: ningún texto recomienda; test que
      falla si aparece «conviene», «mejor» o similar en la salida.
- [ ] CMD-02.3 Evento explícito cuando la amortización agota el saldo (M1).
- [ ] CMD-02.4 `scenario` con N valores · Criterio: cuota y coste total por valor, el valor
      ecoado tal cual y marcado `assumed`.
- [ ] CMD-02.5 e2e: un JSON por comando, códigos 0 y 2, stdout vacío ante error.
- [ ] CMD-02.6 Verificador independiente · Criterio: informe en `odd/reviews/CMD-02.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Decisiones fijadas; implementación no iniciada.

## Siguiente paso

Test en rojo, implementación, verificación.

## Log

### L1 · 2026-10-28 · decisión del usuario

«Sí, con CMD-02». Se acota a `amortize` y `scenario`: `compare` depende de FIN-04, que no
está hecho, y no se expone un comando que no puede cumplir.
