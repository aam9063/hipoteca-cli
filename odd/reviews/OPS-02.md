# Review gate · OPS-02: guarda anti-float, plantillas y cableado de la señal

## Cambio bajo revisión

- **Tarea:** OPS-02 — `odd/tasks/OPS-02-linter.md`.
- **Contenido:** `internal/finance/nofloat_test.go` (guarda ampliada), `cmd/hipoteca/main_test.go`
  (test estructural de la señal), `.github/pull_request_template.md`,
  `.github/ISSUE_TEMPLATE/{bug_report,feature_request}.md`.
- **Riesgo declarado:** medio. No cambia comportamiento de producto; cambia la capacidad de
  detectar un defecto caro.
- **Verificador independiente:** no aplica (no toca dinero ni contrato); verificación del
  autor con intentos de burla, que es lo que corresponde a una guarda.

## El gap que se cierra

Reproducido en vivo antes de tocar nada: con `decimal.NewFromFloat(1.5)`,
`strconv.ParseFloat("3.14", 64)` y `reflect.ValueOf(1.5).Float()` dentro de
`internal/finance`, la guarda anterior (que sólo buscaba los identificadores `float64` y
`float32`) **pasaba en verde**. Una guarda burlable con tres líneas razonables da confianza
falsa sobre el principio más caro del proyecto (P-07).

## Verificación de la guarda nueva

Intentos de burla, en este repositorio, con el resultado real:

| Código introducido en un fichero no-test del paquete | Resultado |
|---|---|
| Los tres casos del verificador | **falla**, 6 hallazgos con posición y motivo |
| `var hexFloat = 0x1p-2` | **falla** (literal float hexadecimal) |
| `var expFloat = 1e10` | **falla** (literal con exponente) |
| `var x float64 = 1` | **falla** (identificador) |
| Comentario y string que dicen `float64` / `ParseFloat` | **pasa** (sin falso positivo) |
| `var entero = 100`, `var medio = 1 << 1` | **pasa** |

El paquete real (`doc.go`, `finance.go`, `schedule.go`) pasa sin excepciones. Los ficheros
de prueba de burla se borraron; el árbol quedó limpio.

## Límites declarados

- La regla de selectores prohíbe **cualquier** nombre que contenga `Float` o `ParseFloat`.
  Es deliberado bajo P-07: en `internal/finance` no hay ninguna API legítima con ese nombre.
  Si alguna vez apareciera, el mensaje explica por qué se rechaza y se discute en su PR.
- El test estructural de la señal **prueba presencia, no comportamiento**: verifica que
  `main.go` llama a `signal.NotifyContext` y usa `ExecuteContext`. Si alguien renombrara el
  paquete importado, la comprobación del calificador podría no verlo. El 130 sigue sin prueba
  e2e porque no existe todavía un comando interrumpible; se acepta y se documenta.

## Veredicto

**PASS.** El gap demostrado está cerrado con evidencia de burla fallida, sin falsos positivos
en el código real, y `bash scripts/check.sh` en verde.
