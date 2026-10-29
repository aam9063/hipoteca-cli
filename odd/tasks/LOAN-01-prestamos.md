# LOAN-01: modelo y comandos del préstamo guardado

## Objetivo

Guardar la hipoteca del usuario en la config local y hacer que los comandos la usen: `loan
add/edit/show/list/remove/sync`, y `revision`/`schedule`/`amortize`/`scenario` funcionando
sobre un préstamo guardado en vez de exigir todas las flags.

## Problema y motivo

Hoy hay que repetir `--principal --rate --months` en cada invocación, y `revision` y
`schedule` responden `loan_incomplete` porque no existe el préstamo. Con la config que CORE-02
dejó (TOML, `0600`, escrituras atómicas) esto es lo que convierte la CLI en algo que se usa a
diario.

## Alcance

- Dentro:
  - Modelo `Loan` con los campos de `docs/ARRANQUE.md` §4.1 y **validación estricta**: los
    datos que dependen de la escritura son obligatorios; si faltan, `loan_incomplete` (código
    2) con un `hint` que nombre el campo.
  - `loan add <id>` con flags, `loan show [<id>]`, `loan list`, `loan edit <id>`,
    `loan remove <id> --yes` (sin `--yes` → `confirmation_required`/2), `loan sync <id>
    --date --outstanding --rate`.
  - `<id>` opcional en todos los comandos cuando hay un único préstamo.
  - `revision` implementado de verdad: mes de referencia (offset de la escritura), índice del
    BdE, `NewRate` con suelo y redondeo, cuota nueva con el capital pendiente y los meses
    restantes, y `--assume` para escenarios (marcado `assumed`).
  - `schedule` desde el préstamo (cuadro completo o tramo `--from/--to`).
  - `amortize`/`scenario` aceptan `<id>` opcional y toman los datos del préstamo.
- Fuera:
  - `compare` (FIN-04) y la mixta completa con histórico de revisiones pasadas (se declara).

## Decisiones

1. **Nunca suponer datos de la escritura** (regla dura del repo): si falta
   `reference_offset_months`, `rate_rounding`, `floor_rate` o `early_repayment_fee_pct`, el
   préstamo está incompleto y el comando dice **qué campo** falta. Un valor por defecto
   inventado produce una cuota que no es la del banco.
2. **`loan sync` manda sobre el cuadro**: si hay `sync`, el capital pendiente y el tipo de
   partida son los del recibo, no los calculados. Es lo que evita arrastrar céntimos durante
   años.
3. **`remove` exige `--yes`** (`confirmation_required`): borrar el préstamo es irreversible
   sobre datos del usuario.
4. **La config se escribe con el mecanismo de CORE-02** (atómico, `0600`); nunca a mano.
5. **`revision` sin el mes publicado no inventa nada**: si el mes de referencia no está en la
   serie, `index_not_published`/4 con el `hint` de `--assume`. Con `--assume`, la salida lo
   marca `assumed` y lo avisa en `warnings`.
6. **Los comandos siguen sin recomendar** (P-08) y toda salida con cálculos lleva sus entradas
   y la atribución.

## Superficie de edición (nuestra)

- `internal/loan/**` (modelo y validación), `internal/config/**` (sección `loans`),
  `internal/app/**` (revision, schedule, wiring de préstamo), `internal/cli/**`
- `odd/tasks/LOAN-01-prestamos.md`, `odd/reviews/LOAN-01.md`

## Riesgo (nuestra)

**Alto**, ítem 1: escribe y borra datos del usuario, y el modelo del préstamo alimenta todo el
dinero. Lleva verificador independiente.

## Checklist

- [ ] LOAN-01.1 Modelo y validación con los campos obligatorios de la escritura; falta un
      campo → `loan_incomplete`/2 nombrando el campo.
- [ ] LOAN-01.2 `add`/`show`/`list`/`edit`/`remove` con round-trip en la config, `remove` con
      `confirmation_required` sin `--yes`.
- [ ] LOAN-01.3 `sync` y su precedencia sobre el cuadro.
- [ ] LOAN-01.4 `<id>` opcional con uno solo; error de uso si hay varios y no se indica.
- [ ] LOAN-01.5 `revision` real contra un índice de los fixtures, con `--assume` marcando
      `assumed`, y `index_not_published` cuando el mes no está.
- [ ] LOAN-01.6 `schedule` desde el préstamo.
- [ ] LOAN-01.7 Verificador independiente · Criterio: informe en `odd/reviews/LOAN-01.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Decisiones fijadas; implementación no iniciada.

## Siguiente paso

Test en rojo, implementación, verificación.

## Log

### L1 · 2026-10-29 · decisión del usuario

«Ahora mismo no te puedo pasar ninguno [recibo], sigue con LOAN-01.» Sin casos reales por
ahora; H-03 queda abierto.
