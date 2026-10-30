# REL-01 · `v0.1.0` instalable en una línea

## Qué se hizo

- **`install.sh`**: detecta plataforma, descarga la release, **verifica el checksum antes de
  extraer y de escribir nada**, y coloca el binario con un `mv` atómico. `--dry-run` muestra
  plataforma, URL y destino sin tocar la red. En Windows sale con 2 y manda a Scoop.
- **`.goreleaser.yaml`**: `brews` y `scoops` con `skip_upload: auto`, para que el release no
  falle mientras el tap y el bucket no existan.
- **`hipoteca update` / `update --check`**: comprueba la última release y reemplaza el binario
  con verificación de checksum y escritura atómica.
- **Job `release-check` en CI**: valida `.goreleaser.yaml` con `goreleaser check` y ejercita el
  instalador con un `uname` falso.
- **README**: las tres vías de instalación, avisando de que sólo la de fuente funciona mientras
  no haya release publicado.

## Las dos garantías que definen la tarea

**El checksum va antes de escribir.** Un instalador que descarga y verifica al final ya
escribió un binario no verificado. Aquí, un checksum que no cuadra aborta con código 5 y la
instalación anterior queda intacta.

**`update` no puede dejar al usuario sin CLI.** Reemplaza el binario **en ejecución**, así que
la verificación y el `mv` atómico no son opcionales. El test
`TestUpdateChecksumMismatchLeavesPreviousBinaryIntact` escribe un «binario-viejo» en el
destino, sirve una release con checksum falso y comprueba que el viejo sigue ahí y que stdout
queda vacío.

## Un defecto que apareció y era el de siempre

Los servidores de prueba de `update` servían un `tar.gz` de **linux** fijo, mientras el CLI
pedía el archivo de **su** plataforma. En Windows eso daba 404, y en el camino feliz «el zip de
la release no contiene hipoteca.exe». Habría fallado el CI de Windows.

Es el quinto defecto de esta familia en el proyecto (bit ejecutable, XDG en macOS, `ENOTDIR`,
`filepath` vs `path` en WATCH-02). La regla que se repite: **un test que construye un valor de
otra plataforma con las herramientas de la suya falla sólo en el CI.**

Arreglo: el servidor sirve todas las plataformas con el hash correcto por extensión, y
`buildFakeZip` mete `hipoteca.exe`.

## El worker se colgó

Entregó T4 (el comando `update`) y se colgó 30 minutos en un `bash`. El resto —`install.sh`,
goreleaser, CI y README— se hizo inline. Es la segunda vez que le pasa lo mismo, y refuerza la
regla de dar timeout a cada invocación.

## Verificación

- `bash scripts/check.sh` → **EXIT=0**.
- `sh -n install.sh` → sintaxis válida.
- `install.sh --dry-run` con `uname` simulado → `hipoteca_0.1.0_linux_amd64.tar.gz` (sin la `v`
  del tag, como lo publica goreleaser).
- Sistema no soportado → exit **2** y mensaje que manda a Scoop.
- `hipoteca update --check` sin releases publicadas → `not_found`/4 con «la release o el fichero
  no existen», no un error interno.
- `go test ./internal/update/ ./internal/cli/` → verde.

## Límites declarados

- **Los dos tests Go del instalador se saltan en Windows** (no hay `sh` nativo). Su primera
  ejecución real será el CI de ubuntu y macos: si el script tiene un fallo de plataforma, se
  verá ahí, no aquí.
- **El tap y el bucket no existen todavía.** `skip_upload: auto` hace que el release publique
  binarios y checksums sin fallar, pero `brew install` y `scoop install` no funcionarán hasta
  que esos dos repositorios existan y el token esté configurado.
- **`update` no se ha probado contra GitHub de verdad**, sólo contra un servidor local. La
  ruta de red real se ejercitará en la primera actualización tras `v0.1.0`.

## Lo que queda, y es del usuario

Mergear a `main` y crear el tag `v0.1.0`. El merge a `main` es decisión de publicación, no de
implementación: `dev` queda verde y con todo lo demás cerrado.
