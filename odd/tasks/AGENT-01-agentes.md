# AGENT-01 · Que un agente resuelva sin contexto

Alcance acotado por la decisión de corte (`WATCH-02-servicio.md`, L3): **sólo las Olas 2 a 3**.
Este documento no cubre `watch`, que pertenece a `v0.2.0`.

## Specs

- **S1.** `hipoteca skills list` lista las recetas disponibles con su id y una línea de para qué sirven, en JSON con `schema_version`.
- **S2.** `hipoteca skills get <id>` devuelve la receta completa —los comandos exactos, en orden, con sus flags— en texto plano listo para ejecutar. Un id desconocido es `not_found`/4 con la lista de ids válidos en el hint.
- **S3.** Las recetas cubren las cuatro preguntas de `docs/ARRANQUE.md` §6.5, y un agente sin contexto puede resolverlas **sólo** con `skills get`, sin leer documentación externa ni adivinar flags.
- **S4.** `hipoteca schema` describe el contrato de salida: los comandos de las Olas 2 a 3, sus campos y tipos, y los códigos de salida. Se genera desde la misma tabla que usan los comandos, no se escribe a mano, para que no pueda quedar desfasado del binario.
- **S5.** `hipoteca doctor` responde en JSON el estado completo: versión y commit, rutas de config y datos con permiso correcto, índices del catálogo, préstamos guardados y cuáles están incompletos y por qué campo, y si la red responde (una sola comprobación, sin tocar el Banco por defecto si `--offline`).
- **S6.** `hipoteca config show|get|set` cubre el hueco declarado en `WATCH-01`: hoy declarar un sink obliga a editar el TOML a mano. `set` valida antes de escribir, con la disciplina atómica y `0600` de `internal/config`.
- **S7.** `doctor` y `config` nunca incluyen un valor secreto en la salida: un sink con credenciales se muestra redactado, igual que en los errores del webhook.

## Tasks

| ID | Tarea | Specs |
|---|---|---|
| T1 | `internal/skills/`: catálogo de recetas como datos embebidos y su render | S1–S3 |
| T2 | `hipoteca skills list|get` | S1, S2 |
| T3 | `hipoteca schema`: generación desde la tabla de comandos | S4 |
| T4 | `hipoteca doctor` | S5, S7 |
| T5 | `hipoteca config show|get|set` | S6, S7 |
| T6 | Tests de contrato: JSON, códigos, redacción de secretos y que `schema` no se desfase | S1–S7 |

## Log

**L1 (pedido del usuario, verbatim):** «sigue lo que dice el documento».

**L2:** alcance fijado a las Olas 2 a 3 para que `REL-01` no cuelgue de la Ola 4. `watch` queda
fuera y se documentará en `v0.2.0`.
