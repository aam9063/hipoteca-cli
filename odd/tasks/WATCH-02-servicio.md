# WATCH-02 · El aviso como servicio

## Specs

- **S1.** `hipoteca watch service install` deja el aviso corriendo como servicio del usuario: unidad de systemd en Linux, agente de launchd en macOS. Es idempotente: instalarlo dos veces no duplica la unidad ni falla.
- **S2.** `hipoteca watch service uninstall` retira todo lo que puso `install`, **sin restos**: la unidad y su estado. Desinstalar dos veces no falla.
- **S3.** `hipoteca watch service status` dice si el servicio está instalado y si está activo, en JSON con `schema_version`.
- **S4.** En un sistema operativo sin servicio de usuario soportado (Windows) las tres acciones son `usage` con un mensaje que explica el límite y sugiere `watch run`, no un error interno.
- **S5.** Sin vigilancias registradas, `install` es `usage`: un servicio que no vigila nada es una trampa silenciosa.
- **S6.** La ruta de la unidad respeta el mismo `DATAPATH`/XDG que el resto (`internal/config`), y la unidad usa la **ruta absoluta del ejecutable** (`os.Executable`), nunca `hipoteca` a secas: un servicio no hereda el PATH del usuario.
- **S7.** El intervalo del servicio es el mismo `--interval` de `watch run`, con el mínimo de una hora ya existente.

## Tasks

| ID | Tarea | Specs | Ruta |
|---|---|---|---|
| T1 | `internal/watch/service.go`: generación pura de la unidad por sistema operativo (contenido y ruta), sin E/S | S1, S6, S7 | worker |
| T2 | `internal/watch/service_install.go`: install/uninstall/status con E/S acotada y `status` en JSON | S1–S3 | worker |
| T3 | `internal/cli/watch.go`: subcomando `service` con las tres acciones y los códigos de salida | S1–S5 | worker |
| T4 | Tests: unidad generada con ruta absoluta, idempotencia, `uninstall` sin restos, Windows rechazado, sin vigilancias rechazado | S1–S7 | worker |

## Log

**L1 (pedido del usuario, verbatim):** «puedo ir mergeando ya a main?» … «vale espero. Sigue pues».

**L2:** el arranque (`docs/ARRANQUE.md:635`) sugiere `v0.1.0` tras las Olas 1 a 3 con los avisos en `v0.2.0`, pero `REL-01` depende de `AGENT-02` → `AGENT-01` → «Olas 2 a 4». Contradicción abierta: decide el usuario antes de REL-01. No bloquea WATCH-02.

**L3 (decisión del usuario, verbatim):** «sigue lo que dice el documento».

Resuelta la contradicción de L2 a favor del arranque: **`v0.1.0` se corta tras las Olas 1 a
3 y los avisos van en `v0.2.0`**. Consecuencias:

- `AGENT-01` se acota a las Olas 2 a 3 (index, calc, revision, schedule, amortize, scenario,
  compare, loan y config): **no documenta `watch`**, que pertenece a `v0.2.0`. Así `AGENT-01 →
  AGENT-02 → REL-01` deja de colgar de la Ola 4.
- Esta tarea (WATCH-02) sigue adelante y se entrega a `dev`, pero **no entra en `v0.1.0`**.
- El corte de `v0.1.0` no puede ser `dev` tal cual, porque `dev` ya contiene los avisos. El
  último commit sin ellos es `1ff3a45` (cierre de la Ola 3). H-03, que valida la Ola 2, es
  posterior: al cortar hay que decidir si entra por cherry-pick en la rama de release o si
  `v0.1.0` sale sin él y el contraste del recibo aparece en `v0.2.0`.
