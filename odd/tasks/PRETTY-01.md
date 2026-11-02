# PRETTY-01 · Salida legible para humanos

Motivo: el README debe enseñar **el comando y su salida real**, como hace `booksy-cli`. Hoy solo
`skills list` tiene `--pretty`; los comandos de cálculo vuelcan una línea de JSON que en un
README no se puede leer y en una terminal tampoco.

## Specs

- **S1.** `--pretty` en `index list`, `revision`, `amortize`, `schedule` y `loan list`: tabla con
  cabecera en mayúsculas y columnas alineadas a la izquierda.
- **S2.** **El JSON sigue siendo el defecto** en todos. `--pretty` es opt-in: es la vista para
  humanos, y el contrato de los agentes no cambia.
- **S3.** El ancho es **fijo** (el mismo de `skills list`), no se lee del terminal: el resultado es
  idéntico en cualquier máquina y los tests no simulan un TTY.
- **S4.** Formato **es-ES** en la tabla (arranque `D-01`): importes con coma decimal y `€`
  (`742,18 €`), tipos con coma y `%` (`3,101 %`), fechas `YYYY-MM-DD` o `YYYY-MM`.
- **S5.** Debajo de cada tabla, **una línea de resumen** con lo que hace falta para interpretarla:
  la atribución y el mes del índice en `revision`, el modo y el ahorro en `amortize`, el número de
  filas y el capital final en `schedule`. Sin esa línea la tabla no se entiende sola.
- **S6.** Las celdas nunca se parten a mitad de palabra; las columnas de texto libre usan el
  envoltorio que ya existe (`wrapLines`), y el resto se trunca con `…` si no cabe.
- **S7.** Un test por tabla: cabeceras, alineación, que el número de filas coincide con el JSON, y
  que sin `--pretty` la salida sigue siendo un único valor JSON.
- **S8.** `docs/cli-spec.md` se regenera con los flags nuevos.

## Tasks

| ID | Tarea | Specs |
|---|---|---|
| T1 | Extraer el renderizador de tablas a un sitio común, sin duplicar `padRight`/`wrapLines` | S1, S3, S6 |
| T2 | `index list --pretty` | S1–S3, S6 |
| T3 | `revision --pretty` con su línea de resumen y atribución | S1–S5 |
| T4 | `amortize --pretty` comparando los modos | S1–S5 |
| T5 | `schedule --pretty` (primeras filas y totales) | S1–S5 |
| T6 | `loan list --pretty` | S1–S3 |
| T7 | Tests y regeneración de `docs/cli-spec.md` | S7, S8 |

## Log

**L1 (pedido del usuario, verbatim):** «me gustaría hacer un readme como el de booksy-cli para
que los ejemplos de uso queden como en ese readme. Si se puede eh, sino lo dejamos así.»

**L2:** el README se reescribe **después** de esto, con las salidas reales. Un README que enseña
un formato que el binario todavía no produce es la misma clase de mentira que el `install.sh` que
pedía un archivo con otro nombre.
