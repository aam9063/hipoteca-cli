# REF-ola5 · Los seis hallazgos del pase de arquitectura

Origen: `odd/reviews/ola-5.md` (QA-01). Ninguno bloqueaba `v0.1.1`; el usuario pidió arreglar los
seis. Orden por valor para el usuario y por riesgo creciente, no por número.

## Specs

### REF-03 · Poder borrar tus datos

- **S1.** `hipoteca uninstall` borra la configuración (con los préstamos), la base de datos y la
  caché, y dice exactamente qué rutas ha eliminado.
- **S2.** **Sin `--yes` no borra nada**: pide confirmación explícita, y sin terminal se niega con
  `confirmation_required`/2 en lugar de asumir un sí.
- **S3.** `--dry-run` (o `--list`) enumera las rutas y su tamaño **sin** borrar nada.
- **S4.** No borra el binario: eso lo hace el gestor de paquetes. La ayuda lo dice.
- **S5.** El README documenta cómo desinstalar por las tres vías y qué se borra.

### REF-06 · Que CI instale de verdad

- **S6.** Un job de CI que, sobre el manifiesto de Scoop del repositorio, **instale el paquete en
  un entorno limpio** y compruebe que el binario responde `version`. No basta con que el archivo
  exista: el fallo que motiva esta tarea fue exactamente ese.
- **S7.** El job falla si el manifiesto apunta a una URL que no existe o a un hash que no cuadra.
- **S8.** Se puede ejecutar a mano (`workflow_dispatch`) sin publicar un release.

### REF-04 · Campos que se guardan y campos que se calculan

- **S9.** Un préstamo puede guardarse sin la comisión de amortización (`fee_pct`) y los comandos
  que **no la usan** (`revision`, `schedule`) deben calcular igual.
- **S10.** Los que **sí la usan** (`amortize`) siguen respondiendo `loan_incomplete` nombrándola,
  con el mismo código de salida y un hint que explique que `0` es un valor válido.
- **S11.** La regla «nada se supone» no se relaja: el campo sigue siendo obligatorio **para lo que
  hace falta**, y no se rellena con un valor por defecto.

### REF-02 · `config` no debe conocer `loan`

- **S12.** `internal/config` deja de importar `internal/loan`. La validación del préstamo vive en
  `loan` (o en `app`), y `config` sólo guarda, carga y valida su propio esquema.
- **S13.** El comportamiento visible no cambia: un fichero con un préstamo inválido sigue siendo
  un error con el mismo mensaje y el mismo código.

### REF-01 · La CLI pasa por `app`

- **S14.** `internal/cli/config.go`, `index.go`, `loan.go` y `watch.go` dejan de importar
  `internal/bde`, `internal/loan` e `internal/watch`: piden a `app`.
- **S15.** Ninguna salida cambia. La suite E2E de cada comando debe seguir verde sin tocarla, y
  `docs/cli-spec.md` no debe cambiar salvo por el texto de ayuda que se añada.

### REF-05 · Partir `compare.go`

- **S16.** `internal/app/compare.go` se divide por responsabilidad (recorrido de históricos y
  presentación del resultado) sin cambiar la firma pública ni una cifra de la salida.

## Tasks

| ID | Tarea | Specs |
|---|---|---|
| T1 | `hipoteca uninstall` con confirmación, `--dry-run` y borrado real | S1–S4 |
| T2 | README: las tres vías de desinstalación y qué se borra | S5 |
| T3 | Job de CI que instala desde el manifiesto e invoca `version` | S6–S8 |
| T4 | Separar campos obligatorios de guardar de campos necesarios para calcular | S9–S11 |
| T5 | Sacar la validación de préstamo de `config` | S12, S13 |
| T6 | Llevar los cuatro archivos de `cli` a `app` | S14, S15 |
| T7 | Partir `compare.go` | S16 |

## Log

**L1 (pedido del usuario, verbatim):** «hazlo en el orden que quieras pero arregla los 6».

**L2:** el orden elegido es REF-03 → REF-06 → REF-04 → REF-02 → REF-01 → REF-05: primero lo que
toca datos personales, después la defensa que faltaba, y al final el refactor puro, que es el
que más superficie mueve y menos cambia lo que ve el usuario.
