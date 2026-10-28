# OPS-01: esqueleto del repositorio

## Objetivo

Dejar un repositorio que compila, se testea y se valida igual en Linux, macOS y Windows,
con las capas de la arquitectura declaradas y las puertas de calidad activas, para que
CORE-01, CORE-02, BDE-01, FIN-01 y OPS-02 puedan trabajar encima sin discutir estructura.

## Problema y motivo

Todo el backlog de la Ola 1 depende de OPS-01. Sin `go.mod`, sin `make check` y sin CI no
hay dónde poner un test en rojo, ni forma de detectar que `internal/finance` se contaminó
con `float64` o que un PR se saltó el review gate.

## Alcance

- Dentro:
  - Módulo `github.com/aam9063/hipoteca-cli` con Go 1.25.
  - `cmd/hipoteca` y el árbol de `internal/` de §8.3, cada capa con su `doc.go`
    declarando la regla que la hace distinta (pureza de `finance`, monopolio HTTP de
    `bde`, contrato de `model`).
  - `make build`, `make check`, `make test`, `make golden`, `make live`, `make tools`.
  - CI en matriz Linux/macOS/Windows con `make build` y `make check`, más el job que
    exige `odd/reviews/<id>.md` cuando el título del PR lleva ID (§12).
  - `AGENTS.md` de §11 tal cual, `CLAUDE.md`, `README.md` con atribución y avisos,
    `LICENSE` MIT, `CHANGELOG.md` y `.goreleaser.yaml`.
  - Un único comando real, `hipoteca version`, en JSON, que sirve de humo y de destino de
    los `ldflags`.
- Fuera:
  - Comandos de producto (`index`, `revision`, `schedule`, ...) y el sobre de error con
    los códigos 0-130: son CORE-01.
  - `cobra` como árbol completo de subcomandos, TOML, decimal y SQLite: entran con las
    tareas que los usan (CORE-02, FIN-01, WATCH-01). Añadir dependencias sin uso rompería
    `go mod tidy`, que el propio check verifica.
  - El job nocturno contra la API real: necesita `internal/bde` (BDE-01).

## Restricciones

- P-01: stdout lleva un único valor JSON; diagnóstico por stderr.
- P-02: los códigos de salida estables viven en `internal/errs` (CORE-01); `main` todavía
  sale con 1 y lo deja anotado.
- P-09: `version` fija su `schema_version: 1` ahora, y solo se amplía.

## Superficie de edición (nuestra)

- `go.mod`, `go.sum`, `Makefile`, `.gitignore`, `.goreleaser.yaml`, `CHANGELOG.md`,
  `LICENSE`, `README.md`, `AGENTS.md`, `CLAUDE.md`
- `cmd/**`, `internal/*/doc.go`, `internal/cli/**`
- `scripts/**`, `.github/workflows/ci.yml`
- `odd/tasks/OPS-01-esqueleto.md`, `odd/reviews/OPS-01.md`

## Riesgo (nuestra)

**Medio-alto.** Toca entrega y entorno (ítem 5 de la lista de riesgo: CI, dependencias de
build) y nada de comportamiento de producto que un test pueda romper (ítem 6). Un CI mal
configurado da falsa confianza: por eso el check no se limita a compilar, y la primera
ejecución real en los tres sistemas es parte del criterio de aceptación.

## Checklist

- [x] OPS-01.1 `go.mod` con Go 1.25 y `cobra` como única dependencia · Criterio:
      `go build ./...` limpio y `go mod tidy` sin cambios.
- [x] OPS-01.2 `cmd/hipoteca` y las 11 capas de `internal/` con `doc.go` que declara su
      regla · Criterio: `internal/finance/doc.go` prohíbe red, E/S, reloj y `float64`;
      `internal/bde/doc.go` reclama el monopolio HTTP.
- [x] OPS-01.3 `hipoteca version` en JSON con `schema_version` · Criterio: test en rojo
      antes de implementar; un solo valor JSON en stdout, verificado por el test.
- [x] OPS-01.4 `make check` = gofmt, `go mod tidy`, vet, staticcheck, govulncheck y tests,
      con las herramientas en `./.bin` · Criterio: corre sin tocar el `GOPATH` del usuario
      y falla si `go.mod` no está tidy.
- [x] OPS-01.5 CI en matriz de tres sistemas + job de review gate · Criterio: el workflow
      corre `make build` y `make check` en los tres y falla si un PR con ID no trae
      `odd/reviews/<id>.md`.
- [x] OPS-01.6 Documentos del repositorio · Criterio: `AGENTS.md` reproduce §11,
      `README.md` incluye atribución dual, "no es asesoramiento" y "no afiliado", y
      `LICENSE` es MIT.
- [x] OPS-01.7 Verificación local de `make check` · Criterio: verde en este Windows.
- [x] OPS-01.8 Verificación en CI de los tres sistemas · Criterio: los tres jobs en verde
      en el PR. Run `40690111332`: ubuntu, macos y windows en success.
- [x] OPS-01.9 Review gate con informe en `odd/reviews/OPS-01.md` · Criterio: PASS tras la
      única ronda de correcciones, con la evidencia de los dos modos de fallo del `tidy`.

## Evidencia de verificación

- `bash scripts/check.sh` en local (Windows, Go 1.26.1): **EXIT=0**. gofmt, `go mod tidy`
  sin diff, vet, staticcheck, govulncheck (0 vulnerabilidades en código llamable) y tests.
- CI, run `40689508544`: `check (windows-latest)` **success**; `check (ubuntu-latest)` y
  `check (macos-latest)` **failure** con `scripts/tools.sh: Permission denied`.
  Causa: el bit ejecutable nunca entró al índice de git porque el autor trabaja en Windows,
  donde no existe. Corregido en `f47db0f` (modo 100755 en los cuatro scripts, invocación
  por `bash` en `check.sh`, y `*.sh text eol=lf` en `.gitattributes` para que un shebang con
  CRLF no rompa Unix).
- CI, run `40690111332` (con `f47db0f`): **ubuntu, macos y windows en success**.
- Reproducción del review gate: 7 títulos probados; detecta `OPS-01`, `FIN-04`, `CORE-01`
  sin informe y no se activa con títulos sin ID.
- `make build` produce `bin/hipoteca` y `hipoteca version` emite
  `{"schema_version":1,"version":"afdda45-dirty",...}`: los `ldflags` tienen destino real.

### Nota sobre Windows y `make`

El review gate levantó la duda de si `make` existe en `windows-latest`. Los hechos la
refutan: el job `check (windows-latest)` pasó ejecutando `make build` y `make check` en el
run `40689508544`. No se añade ninguna mitigación para un problema que no ocurre.

## Progreso

**Cerrada.** PR #3 mergeado a `dev` con `--rebase`; run `40691442162` con los cuatro jobs
en verde (`check` en ubuntu/macos/windows y `review gate report`).

### Hashes

El merge con rebase reescribe los ids, así que el informe de review cita los de la rama
(el rango revisado) y aquí queda la correspondencia real con `dev`:

| Revisado (rama) | En `dev` | Contenido |
|---|---|---|
| `7e408b9` | `8ace4f4` | build: bootstrap del módulo y las capas |
| `2d83aee` | `1385328` | feat(cli): `version` con contrato JSON |
| `107c97f` | `77d9789` | build(check): entrada portable del check |
| `77fbd2d` | `10d751c` | docs: documentos del repositorio |
| `24a6c5f` | `6013091` | ci: matriz de tres sistemas |
| `f47db0f` | `9b20b47` | fix(ci): bit de ejecución de los scripts |
| `ac1bb75` | `a4be17a` | fix(cli,ci): ronda única de review |
| `fa9579d` | `eca15ed` | docs(ops-01): informe de review y evidencia de CI |

La evidencia final es el run `40691442162` sobre `eca15ed`: los tres `check` y el
`review gate report`, todos en success.

## Siguiente paso

Ola 1 continúa con CORE-01, CORE-02 y FIN-01 (superficies disjuntas). FIN-01 necesita
SPIKE-02 (`docs/calculos.md` + casos de referencia) y BDE-01 depende de CORE-02.

## Log

### L1 · 2026-10-28 · decisión del usuario

Tras cerrar SPIKE-01, el usuario elige "OPS-01: esqueleto del repo" como siguiente paso.

### L2 · 2026-10-28 · decisión técnica

Este Windows no tiene `make`. En vez de instalar software en la máquina del usuario, el
Makefile queda como interfaz delgada y `scripts/*.sh` como implementación, que es lo que
corre la CI. `make check` y `bash scripts/check.sh` hacen exactamente lo mismo.

### L3 · 2026-10-28 · correcciones del review gate

El review de contexto limpio devolvió PASS CON FIXES. Se aplica **una sola ronda**:

- El check restaura `go.mod`/`go.sum` con un `trap` cuando `go mod tidy` falla.
- `hipoteca` sin subcomando manda la ayuda a **stderr** y falla (P-01), y se desactiva el
  comando `completion` que cobra añade por defecto y que escribiría shell en stdout.
- `govulncheck` queda fijado a una versión concreta (el check debe ser reproducible).
- La detección de ID del review gate pasa a ser insensible a mayúsculas.
- Se elimina el filtro muerto de `gofmt -l` si se demuestra que lo es.

Descartado con evidencia: el hallazgo sobre `make` en `windows-latest` (el job pasó).
Aceptado como transitorio: `exit 1` en `main`, que CORE-01 sustituye por los códigos del
arranque §6.4.

### L4 · 2026-10-28 · cierre

CI final `40691442162` en verde (los tres checks y el review gate). PR #3 mergeado a `dev`
con `--rebase --delete-branch`. El merge reescribe hashes: los informes de review citan el
rango revisado y el PR, y la correspondencia con `dev` se anota al cerrar.
