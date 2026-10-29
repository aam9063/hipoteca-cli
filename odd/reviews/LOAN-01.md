# Review gate · LOAN-01: préstamos guardados y revisión real

## Cambio bajo revisión

- **Tarea:** `odd/tasks/LOAN-01-prestamos.md`.
- **Rama:** `hc/loan-01-prestamos` (base `dev`).
- **Riesgo declarado:** ítem 1 — escribe y borra datos del usuario y alimenta todo el dinero.
- **Verificador independiente:** sí, con el binario real y la caché sembrada desde los fixtures
  (sin tocar la red).

## Veredicto del verificador: PASS WITH FIXES (1 blocker, 3 minors)

Lo que confirmó con datos reales:

- **Nunca se supone un campo de la escritura**: faltando `reference_offset_months`,
  `rate_rounding`, `floor_rate` o `early_repayment_fee_pct`, todos los comandos dan
  `loan_incomplete`/2 nombrando el campo y con hint a `loan edit`. Y
  `--reference-offset-months 0` se acepta como valor legítimo (el modelo distingue "cero" de
  "ausente").
- **Sin pérdida silenciosa**: un TOML corrupto produce error con la ruta y el archivo queda
  **byte-idéntico** (verificado con md5); `schema_version` desconocida también se rechaza.
- **`remove` sin `--yes`** devuelve `confirmation_required`/2 y el préstamo sigue legible.
- **`sync` manda**: tras sincronizar, el cuadro usa `3.12 %` y `152341.18`, no los calculados.
- **`revision`**: mes de referencia con cambio de año, cadena spread→suelo→redondeo en orden,
  `index_not_published`/4 sin `--assume`, `status: assumed` con aviso de P-06 cuando se usa, y
  los pagos contrastados con una implementación Python independiente.
- **`schedule`** de un préstamo fijo **idéntico al caso 1** de referencia.

## El blocker: un hint que apuntaba a un comando roto

`loan edit casa --reference-offset-months 2` fallaba con «el tipo "" no es correcto», porque
validaba el préstamo construido **sólo con las flags** (donde `--type` está vacío) en vez del
fusionado con el guardado. Y el `hint` de `loan_incomplete` dice literalmente «completa el campo
con hipoteca loan edit»: **el camino de recuperación que el propio error recomienda no
funcionaba**. Un hint circular es peor que ninguno.

Corregido: `edit` parsea las flags, las fusiona sobre el préstamo guardado y valida **el
resultado**; `add` conserva la validación previa a tocar la config. Verificado por el autor:
`edit --reference-offset-months 3` cambia el campo sin repetir `--type` y el tipo sigue siendo
`variable`; un valor inválido sigue dando `usage`/2 y no escribe nada.

## Los tres menores, corregidos

| Hallazgo | Corrección |
|---|---|
| `warnings` salía `null` cuando estaba vacío, contra el ejemplo §5.2 | Slice vacío no nulo en el camino de éxito, con aserción |
| `loan remove` exigía id aunque hubiera un solo préstamo, cuando sus hermanos lo aceptan omitido (§5.1) | Resuelve el id igual que los demás; `--yes` sigue siendo obligatorio y sin él no se toca nada |
| El hint decía «los préstamos guardados llegan con LOAN-01», y LOAN-01 es esta tarea | Texto actualizado |

## Verificación del autor

- `bash scripts/check.sh` → **EXIT=0**.
- Con `HOME` temporal: `add` persiste `config.toml`; `loan_incomplete` nombra el campo;
  `schedule` produce el cuadro con fechas y `582.45`/`156.25`; `revision 2022-06 --assume 0.50`
  devuelve `reference_month 2022-04`, `raw 1.39`, cuota `591.81`, diferencia `9.36` y el aviso
  de P-06; `edit` de un solo campo funciona; `edit --type loca` falla con `usage`/2.

## Límites declarados (advisories del verificador)

- Un préstamo **fijo sin `sync`** no puede ejecutar `revision` más allá del primer mes, porque
  el comando se niega a inventar el capital pendiente. Para un fijo el cuadro es exacto, así
  que es conservador de más; se revisará con FIN-04.
- `amortize --date` es un índice de cuota, no una fecha: contrato de CMD-02.
- El worker que implementó la tarea **se colgó 30 minutos** en un `bash` y dejó dos lints;
  no dejó código roto. El flujo de verificación necesita un timeout propio.
- H-03 (recibos reales) sigue abierto.

## Veredicto

**PASS tras la ronda única de correcciones.** El préstamo incompleto no calcula y dice qué
falta; la recuperación que ese error promete funciona; y los datos del usuario nunca se pierden
en silencio.
