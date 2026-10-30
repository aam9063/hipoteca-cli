# REL-01 · `v0.1.0` instalable en una línea

Alcance: **todo `dev`**, avisos incluidos por la decisión de corte del usuario (A).

## Specs

- **S1.** `install.sh` instala el binario en una línea: detecta sistema operativo y arquitectura, descarga la release de GitHub, **verifica el checksum antes de instalar** y coloca el binario en `bin`. Un checksum que no cuadra aborta sin dejar nada escrito.
- **S2.** `install.sh` es determinista y testeable sin red: la detección de plataforma y la construcción de la URL se pueden ejercitar con `--dry-run`, y un sistema o arquitectura no soportados fallan con un mensaje que dice cuáles sí lo están.
- **S3.** `.goreleaser.yaml` publica `checksums.txt`, los archivos de cada plataforma y el changelog, y está **validado en CI** (`goreleaser check`), porque en la máquina de desarrollo no está instalado: un archivo de release que nadie valida falla el día del tag.
- **S4.** `brews` (tap de Homebrew) y `scoops` (bucket) quedan configurados apuntando a los repositorios del proyecto, y su publicación no rompe el release cuando esos repositorios o el token todavía no existen.
- **S5.** `hipoteca update` comprueba la última release publicada y, si el usuario lo pide, reemplaza el binario **con verificación de checksum y escritura atómica**: un fallo de red, un checksum que no cuadra o un binario corrupto dejan la instalación anterior intacta y funcionando.
- **S6.** `update --check` no modifica nada y responde en JSON con la versión actual, la disponible y si hay actualización.
- **S7.** El README documenta las tres formas de instalar (script, tap, Scoop) y cuál es la recomendada, sin prometer lo que no exista en el momento de la release.
- **S8.** Ninguna prueba de PR toca la red: las pruebas de `install.sh` y de `update` usan un servidor local o un runner inyectado.

## Tasks

| ID | Tarea | Specs |
|---|---|---|
| T1 | `install.sh` con detección, descarga, checksum y `--dry-run` | S1, S2, S8 |
| T2 | `.goreleaser.yaml`: `brews`, `scoops` y tolerancia a que no existan aún | S4 |
| T3 | Job de CI que valida `.goreleaser.yaml` y ejercita `install.sh --dry-run` | S3 |
| T4 | `internal/update` + `hipoteca update [--check]` con reemplazo atómico | S5, S6, S8 |
| T5 | README: las tres vías de instalación | S7 |

## Log

**L1 (pedido del usuario, verbatim):** «sigue lo que dice el documento».

**L2 (decisión de corte, elegida por el usuario):** «A — Incluir los avisos en v0.1.0». La
sugerencia original del arranque quedaba desactualizada: separar los avisos exigía reescribir
historial y publicar código que ningún CI habría probado en esa combinación. `v0.1.0` sale de
`dev` completo.

**L3:** `hipoteca update` es la pieza de más riesgo de esta tarea porque **reemplaza el binario
en ejecución**. La verificación de checksum y la escritura atómica no son opcionales, y su
verificador independiente debe intentar corromper una descarga y comprobar que la instalación
anterior sobrevive.
