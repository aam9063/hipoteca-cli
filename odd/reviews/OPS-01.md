# Review gate · OPS-01: esqueleto del repositorio

## Cambio bajo revisión

- **Tarea:** OPS-01 — esqueleto del repositorio (`odd/tasks/OPS-01-esqueleto.md`).
- **Rango congelado:** `dev..HEAD` (6 commits):
  - `7e408b9` build: bootstrap the Go module and declare the architecture layers
  - `2d83aee` feat(cli): add the version command with a JSON output contract
  - `107c97f` build(check): add the portable check entry point
  - `77fbd2d` docs: add the repository documents
  - `24a6c5f` ci: run build and check on Linux, macOS and Windows
  - `f47db0f` fix(ci): make the check scripts executable and stop relying on the mode
- **Riesgo declarado:** medio-alto (toca entrega y entorno; no toca `internal/finance`).
- **Revisión:** subagente revisor de contexto limpio, solo lectura. No corrió ni el
  `assess` nativo ni un verificador independiente: la herramienta nativa devolvió
  `nativeRisk: unassessable` con motivo `package-local-binary-missing`, y el verificador
  independiente no está admitido aquí porque la tarea no nombra ningún ítem de riesgo alto
  (el riesgo alto reservado al verificador es `internal/finance`; esta tarea lo tiene fuera
  de alcance). No se inventa evidencia de terceros: todo lo que sigue es observado o
  referenciado con su identificador.

## Verificación ejecutada

### Ronda 1 (informe inicial)

- `git log --oneline dev..HEAD` → 5 commits; `git status --short` → árbol limpio.
- `bash scripts/check.sh` → **exit 0** («==> ok», govulncheck: «0 vulnerabilities», test de
  `internal/cli` ok).
- `go run ./cmd/hipoteca` → texto de ayuda en stdout, exit 0 (H-2).
- `go run ./cmd/hipoteca version` → `{"schema_version":1,"version":"dev","commit":"none","date":"unknown"}`,
  una única línea JSON, exit 0.
- `go run ./cmd/hipoteca version extra` → error en stderr, **exit 1** (H-5).
- `go run ./cmd/hipoteca completion bash` → script de shell en stdout, exit 0 (H-2).
- Regex del review gate: `OPS-01`/`SPIKE-01`/`FIN-04`/`REF-12` detectados; `ops-01`
  minúsculas sin coincidencia (H-6).
- Ruta de fallo del `go mod tidy` reproducida en copia temporal: exit 1 con
  `go.mod.check.bak` y `go.sum.check.bak` abandonados en el árbol (H-1).
- `ls testdata/bde/` → 13 capturas JSON + README (afirmación del CHANGELOG cierta);
  `internal/bde/series.yaml` → 14 índices + 3 series auxiliares; `docs/series.md` §8 existe.
- No observado en esta máquina: `goreleaser check` (binario no instalado) y `make` (este
  Windows no lo tiene; el check se validó vía `bash scripts/check.sh`).

### Ronda 2 (correcciones aplicadas; esta es la evidencia vigente)

- `go test ./internal/cli/` **antes** del fix → RED observado:
  `--- FAIL: TestRootWithoutSubcommandKeepsStdoutEmpty` y
  `--- FAIL: TestCompletionCommandIsDisabled` (exit 1).
- `go test ./internal/cli/` **después** → `ok github.com/aam9063/hipoteca-cli/internal/cli`,
  exit 0 (el test de `version` original sigue en verde).
- `go build ./...` → ok.
- Binario real: `go run ./cmd/hipoteca` → stdout **vacío**, exit 1, ayuda y error en stderr;
  `go run ./cmd/hipoteca completion bash` → `unknown command "completion" for "hipoteca"`.
- Ruta de fallo del tidy probada en dos copias desechables fuera del árbol
  (`git archive HEAD` + los `scripts/` corregidos):
  - **Caso A** (`go.mod` sintácticamente roto, `go mod tidy` falla): exit 1, mensaje
    «go mod tidy failed: go.mod/go.sum restored to the committed state», `go.mod`
    **byte-idéntico** al estado previo (sha256 comparado) y **sin** `*.check.bak`.
  - **Caso B** (`go.mod` válido pero no tidy, `tidy` lo cambiaría): exit 1, mensaje
    «go mod tidy changed go.mod/go.sum: commit the tidy result (go.mod/go.sum restored)»,
    `go.mod` y `go.sum` byte-idénticos al estado previo (tidy revertido) y sin `.bak`.
- `scripts/check-review-report.sh` probado con títulos reales: «OPS-01: esqueleto» → exige
  y encuentra el informe (exit 0); «ops-01 en minúsculas» → detectado tras normalizar
  (exit 0); «Fix FIN-04 y CORE-01» → exige el primero (`FIN-04`, exit 1 sin informe);
  «merge hc/OPS-02-review-gate into dev» → token con barra ignorado, gate saltado
  (exit 0); «ola 2 sin id» → saltado (exit 0); «SPIKE-01 contrato API» → informe presente.
- `bash scripts/check.sh` final → **exit 0** («==> ok», test de `internal/cli` ok).

## Hallazgos de la ronda 1 y su resolución

| Sev. | Archivo:línea (original) | Qué estaba mal | Resolución |
|---|---|---|---|
| major | `scripts/check.sh:18-27` | Con `set -e`, un `go mod tidy` que falla salía antes de restaurar: quedaban `go.mod.check.bak`/`go.sum.check.bak` y el árbol podía quedar mutado. | **Corregido.** `trap restore_module_files EXIT` mientras existan los `.bak`; el mensaje de fallo es explícito y la salida siempre deja `go.mod`/`go.sum` como estaban. Probado en los casos A y B de la ronda 2. |
| major | `internal/cli/root.go:32-34` | Sin subcomando, la ayuda iba a **stdout** (P-01 roto) y `completion` de cobra escribía shell en stdout. | **Corregido.** Sin subcomando: ayuda a stderr (`cmd.SetOut(cmd.ErrOrStderr())`), error y exit 1, stdout vacío; `CompletionOptions.DisableDefaultCmd = true`. Dos tests nuevos en `internal/cli/root_test.go`, en rojo antes del fix. Decisión registrada: `hipoteca --help` explícito sigue el comportamiento estándar de cobra (texto en stdout, exit 0), fuera del alcance de esta corrección. |
| major | `.github/workflows/ci.yml:32,35` | `make` en `windows-latest` no garantizado (no verificable localmente). | **Refutado por evidencia.** CI run 40689508544: `check (windows-latest): success` ejecutando `make build` y `make check`. No se cambia CI ni Makefile. La causa raíz del rojo original fue el bit de ejecución nunca grabado desde Windows, resuelto en `f47db0f` (todos los scripts en modo 100755 en el índice, `check.sh` invoca `scripts/tools.sh` vía `bash`, y `.gitattributes` fuerza `eol=lf` para `*.sh`). |
| minor | `scripts/tools.sh:20` | `govulncheck@latest` sin fijar: check y hooks de release no reproducibles. | **Corregido.** Fijado `@v1.9.0` (la versión que ya corría en esta máquina, verificada con `./.bin/govulncheck -version`), mismo estilo que `staticcheck@2025.1.1`. |
| minor | `cmd/hipoteca/main.go:25` | `os.Exit(1)` para todo error, incluido `usage` (§6.4 reserva el 2). | **Aceptado, sin cambio** (transitorio y anotado en el código; CORE-01 lo sustituye en su PR, sin tests que fijen el 1). |
| minor | `scripts/check-review-report.sh:12` | IDs en minúsculas saltaban el gate en silencio; tokens estilo rama (`hc/OPS-01-...`) contaban como ID. | **Corregido.** Normalización con `tr '[:lower:]' '[:upper:]'`; un token solo cuenta si es exactamente `PREFIJO-NÚMERO` y no contiene barra; se exige el informe del primer ID. Comportamiento verificado título a título (ver ronda 2). |
| minor | `.goreleaser.yaml:7-8,18-22` | Hooks de release acoplados a `govulncheck@latest`; `goreleaser check` no corrible aquí. | **Aceptado, sin cambio en el fichero.** Con la fijación de H-4 el riesgo de deriva desaparece; correr `goreleaser check` en REL-01. Sintaxis v2 y ldflags (`main.version`, `main.commit`, `main.date`) coherentes con `main.go` por inspección. |
| nit | `scripts/check.sh:15` | Filtro `grep -v '^\.bin/'` muerto: `.bin/` solo contiene binarios instalados por `tools.sh` (verificado con `ls .bin/`), nunca `.go`. | **Corregido:** filtro eliminado, con comentario que documenta por qué es seguro. |
| nit | `internal/cli/version_test.go` | El contrato de `version` vive solo en el test, sin fichero en `schemas/`. | **Aceptado, sin cambio:** difiere a CORE-01, que definirá el directorio. |

## Comprobaciones que dieron bien (sin hallazgo)

- **Veracidad documental:** todas las afirmaciones comprobadas del README/CHANGELOG son
  ciertas hoy: 14 índices en `series.yaml` (más 3 auxiliares), 13 capturas reales en
  `testdata/bde/`, `docs/series.md` §8, atribución, «no es asesoramiento» y «no está
  afiliado ni respaldado» en README (P-05/P-08). `make golden` es un no-op anotado en el
  Makefile («implemented with the output contract, CORE-01») y el README no lo anuncia.
- **Aislamiento de herramientas:** `tools.sh` instala en `./.bin` con `GOBIN` propio, sin
  tocar el `GOPATH`.
- **CRLF:** con `f47db0f`, `.gitattributes` fuerza `* text=auto eol=lf` y `*.sh text
  eol=lf`: ni `gofmt` ni los scripts se ven afectados por `core.autocrlf=true` en Windows.
- **Salida de errores del script:** `set -euo pipefail` + `|| true` correctos en gofmt y en
  la extracción del ID; gofmt, vet, staticcheck, govulncheck y tests sí saltan el exit.
- **P-09:** `version` fija `schema_version: 1` y lo tiene testado.

## Veredicto

**PASS** (tras la única ronda de correcciones que admite §10.5).

Los dos majors reales quedaron corregidos con evidencia (H-1 restauración por `trap`
probada en los dos modos de fallo; H-2 stdout limpio con tests en rojo→verde), los minors
aplicables corregidos, y el tercero refutado por la corrida real de CI (run
40689508544). `bash scripts/check.sh` cierra en exit 0. Principios en riesgo restantes:
ninguno bloqueante; P-02 sigue siendo transitorio por diseño hasta CORE-01.

**Cierre del pendiente de CI:** la corrida `40690111332`, sobre el commit `f47db0f` (el
que corrige el bit de ejecución), da los tres jobs en `success`: `check (ubuntu-latest)`,
`check (macos-latest)` y `check (windows-latest)`. La corrida `40689508544` acreditaba
solo Windows porque las otras dos fallaban por el permiso de los scripts. Con esto
OPS-01.8 queda verificado en los tres sistemas, no inferido.

El único job en rojo de esa corrida es `review gate report`, y lo está por diseño: exige
`odd/reviews/OPS-01.md`, que entra en el commit siguiente a esta corrida.

## Pendiente para el usuario

1. Merge a `dev`. En este repositorio el agente commitea, pushea, abre el PR y mergea
   contra `dev` por instrucción explícita del usuario; el merge a `main` sigue siendo
   humano (H-02).
2. En REL-01: `goreleaser check` en una máquina con el binario antes del primer tag.
3. Decisiones registradas, no requieren acción: `--help` explícito sigue en stdout
   (estándar cobra); `hipoteca help` anida el mismo comportamiento; el sobre de error JSON
   y los códigos estables entran con CORE-01.
4. Limpieza opcional: las pruebas de los casos de fallo se hicieron en directorios
   temporales del sistema bajo `/tmp/gp99R2F8/`; quedan fuera del repositorio.
