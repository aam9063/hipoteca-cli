# Review gate · CORE-01: salida, sobre de error, códigos y atribución

## Cambio bajo revisión

- **Tarea:** CORE-01 — `odd/tasks/CORE-01-salida.md`.
- **Rama:** `hc/core-01-salida` (base `dev`).
- **Riesgo declarado:** alto, ítem 3 (el sobre y los códigos son contrato que consumen
  agentes y scripts).
- **Verificador independiente:** sí. Escalado el ítem 3 y aceptado por el plan
  (`independentVerifier: true`), ejecutado por un subagente de contexto limpio y sólo
  lectura.

## Historial del gate: FAIL y cambio de diseño

El primer veredicto fue **FAIL**, y fue correcto. La implementación inicial serializaba los
decimales con un reescritor reflexivo de tipos (222 líneas, `reflect.StructOf`, caché y
fallbacks). El verificador encontró, con casos reproducidos:

| Sev. | Hallazgo |
|---|---|
| **blocker** | Un `decimal.Decimal` guardado en un campo de interface no vacía (`struct{V fmt.Stringer}`) se serializaba **recortado en silencio**: `3.10` → `3.1`, sin error. Exactamente la degradación que la decisión 3 prohíbe, por una puerta no prevista. |
| **major** | `panic` al reconstruir un struct con un campo interface que contiene un decimal. |
| **major** | `panic` con un puntero `nil` a un struct con decimales (`encoding/json` habría emitido `null`). |
| **major** | `hipoteca --help`, `help` y `version --help` escribían prosa en **stdout** con exit 0 (cobra maneja la ayuda antes de `RunE`). |
| **major** | `version extra-arg` salía con código **1** (`internal`) en vez de 2 (`usage`): `cobra.NoArgs` falla antes de los envoltorios del root. |
| minor | Invocación desnuda: el help completo y el sobre JSON quedaban concatenados en stderr, que deja de ser parseable. |
| minor | `version` escribía con `json.Encoder` en vez de `output.WriteJSON`: dos escritores. |
| nit | `RawDataAttribution("")` aceptaba una fecha vacía. |

**Decisión de diseño, no de parche:** se eliminó la reflexión. Tres defectos distintos en la
misma maquinaria (uno silencioso y dos panics) indican que el enfoque es el problema. Se
reemplazó por `model.Decimal`, un tipo explícito con `MarshalJSON` de escala exacta, y una
guarda de AST que impide declarar un campo `decimal.Decimal` en los tipos de salida.
`WriteJSON` pasó de 382 a **26 líneas** y su comportamiento es, por construcción, el de
`encoding/json`.

## Verificación del autor sobre la corrección

Los mismos ocho probes del verificador, con sus códigos y bytes:

| Invocación | exit | stdout | stderr |
|---|---|---|---|
| `version` | 0 | JSON (1 valor) | 0 B |
| `--help` / `help` | 0 | prosa (excepción documentada) | 0 B |
| `version --help` | 0 | prosa | 0 B |
| `inventado` | 2 | **0 B** | sobre `usage`/2 |
| `version --bogus` | 2 | **0 B** | sobre `usage`/2 |
| `version extra` | **2** | **0 B** | sobre `usage`/2 |
| *(sin subcomando)* | 2 | **0 B** | **un único** JSON parseable |

- El stderr de la invocación desnuda parsea como JSON y trae `type`, `message`, `exit_code`
  y `hint`: ya no hay prosa concatenada.
- `output.go` quedó sin reflexión (no hay `reflect` en el código; sólo la palabra en un
  comentario).
- `bash scripts/check.sh` → **EXIT=0**.
- `go test ./...` verde, incluidos los doce códigos de la tabla, la forma del sobre, la
  omisión de `hint` vacío, y el camino `ctx cancelado → 130` sin sobre.

## Decisión 7: la ayuda explícita es la excepción

`--help`, `help` y `<comando> --help` escriben texto legible en stdout con exit 0. Se
documenta como excepción única al contrato: los **resultados** son JSON; la ayuda es
documentación y una persona espera poder paginarla (`hipoteca --help | less`). Sin
subcomando, en cambio, no se imprime la ayuda: sólo el sobre con el `hint` de `--help`, para
que stderr siga siendo parseable.

## Límites que quedan escritos

- **La exactitud decimal ya no es global.** `WriteJSON` serializa lo que recibe; la garantía
  vive en `model.Decimal` y en la guarda de AST, que hoy sólo cubre `internal/model` (los
  demás paquetes aún no tienen structs de salida). Cuando CORE-02 y los comandos de la Ola 2
  añadan structs de salida, la guarda debe extenderse a esos paquetes en su PR.
- **El código 130 no tiene prueba e2e**: no existe todavía un comando lo bastante largo para
  interrumpirlo. Está cubierto por test unitario de `exitCodeFor`.
- Los códigos 4, 5, 6 y 7 se prueban por tipo; su disparo real llega con BDE-01 y LOAN-01.

## Veredicto

**PASS tras la ronda única de correcciones.** El blocker y los dos panics desaparecieron con
el cambio de diseño; los cinco hallazgos restantes están corregidos y verificados con los
probes del verificador, no con los míos.
