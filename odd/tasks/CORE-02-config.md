# CORE-02: configuración local, rutas XDG y escrituras atómicas

## Objetivo

Guardar y leer la configuración local del usuario de forma segura y portable: rutas según
XDG, permisos `0600` y escrituras atómicas que no puedan dejar el archivo a medias.

## Problema y motivo

La configuración contiene los datos de la hipoteca del usuario y los préstamos guardados. Una
escritura no atómica que se interrumpe deja el archivo truncado y el usuario pierde sus datos:
eso no se reconstruye. Es la única pieza del proyecto que puede destruir información del
usuario, y por eso se trata como riesgo alto.

## Alcance

- Dentro:
  - Resolución de las tres rutas: config, datos y caché, con `XDG_*` cuando existen y
    fallback portable (incluido Windows, donde XDG no existe).
  - `Load`/`Save` de `config.toml` con `schema_version`, overrides de códigos de serie y un
    espacio para lo que traerán LOAN-01 y WATCH-01.
  - Escritura atómica: temporal en el mismo directorio, permisos `0600`, `Sync` y `Rename`.
  - Tests con `HOME` temporal (`t.Setenv`) y verificación de permisos en Unix.
- Fuera:
  - El modelo del préstamo (`[loans.*]`): es LOAN-01. Aquí sólo se reserva el espacio.
  - La base de datos SQLite de vigilancias: es WATCH-01.

## Decisiones

1. **Rutas con la stdlib cuando se puede.** Config con `os.UserConfigDir()` (respeta
   `XDG_CONFIG_HOME` en Unix y `%AppData%` en Windows). Caché con `os.UserCacheDir()`.
   Datos: `XDG_DATA_HOME` si está, si no el fallback de la plataforma, porque la stdlib no
   tiene `UserDataDir`.
2. **Nombres exactos del arranque §7**: `hipoteca-cli/config.toml`, `hipoteca-cli/hipoteca.db`,
   `hipoteca-cli/` para la caché.
3. **`0600` en el archivo y `0700` en el directorio**. En Windows el bit no aplica: se
   documenta y el test de permisos se salta en esa plataforma.
4. **Atomicidad**: escribir el temporal en el **mismo directorio** (mismo sistema de archivos,
   si no `Rename` no es atómico), `Sync` antes de cerrar, `Rename` encima del destino. Nunca
   escribir el archivo final directamente.
5. **Config ausente no es error**: `Load` devuelve la configuración por defecto. Config
   corrupta **sí** es error (código 1 `internal`), y nunca se sobrescribe sola: perder datos
   del usuario por "recuperar" automáticamente es peor que fallar.
6. **Los overrides de series no rompen el catálogo**: se validan contra `internal/bde`
   cuando exista BDE-01; aquí sólo se guardan como mapa `id → código` y se documenta.

## Superficie de edición (nuestra)

- `internal/config/**`
- `go.mod`, `go.sum` (añadir `go-toml/v2`)
- `odd/tasks/CORE-02-config.md`, `odd/reviews/CORE-02.md`

## Riesgo (nuestra)

**Alto, ítem 1**: una escritura no atómica o un `Load` que "arregla" el archivo pueden
destruir datos que el usuario no puede reconstruir. Lleva verificador independiente.

## Checklist

- [ ] CORE-02.1 Rutas XDG con fallback portable · Criterio: con `XDG_CONFIG_HOME` y `HOME`
      temporales las rutas son las esperadas; en Windows usa `%AppData%`; probado en los tres
      SO por CI.
- [ ] CORE-02.2 `Load` con config ausente → valores por defecto; config inválida → error
      explícito sin sobrescribir.
- [ ] CORE-02.3 `Save` atómico · Criterio: el archivo destino nunca queda a medias; los
      permisos son `0600`; un fallo a mitad no deja el destino corrupto.
- [ ] CORE-02.4 Round-trip · Criterio: guardar y volver a leer devuelve lo mismo, incluidos
      los overrides de series.
- [ ] CORE-02.5 Directorio creado con `0700` si no existe.
- [ ] CORE-02.6 Verificador independiente · Criterio: informe en `odd/reviews/CORE-02.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Implementado y verificado: rutas, Load/Save y escritura atómica con tests
(RED → GREEN). Pendiente: verificador independiente (CORE-02.6) y review gate.

## Siguiente paso

Review gate con informe en `odd/reviews/CORE-02.md`.

## Log

### L1 · 2026-10-28 · decisión del usuario

"Cierra cualquier gap y seguimos." Cerrados los gaps con OPS-02 (PR #8), se continúa con
CORE-02, que es la dependencia de BDE-01.

### L2 · 2026-10-28 · implementación

Test-first en `internal/config`: RED observado (build falla: `undefined: Config`,
`undefined: writeFileAtomic`, …), luego GREEN. Biblioteca TOML:
`github.com/pelletier/go-toml/v2` v2.4.3, disponibilidad confirmada con `go get` (la
tabla de arranque §8.1 dice "go-toml/v2": es esta). Justificada en `doc.go`. Windows:
`os.UserConfigDir`/`os.UserCacheDir` ignoran las XDG_*; los tests lo cubren con
`%AppData%`/`%LocalAppData%` y saltan las aserciones XDG/HOME de Unix, con comentario.
`XDG_DATA_HOME` se respeta en las tres plataformas (decisión 1) con fallback
`~/.local/share` (Unix) y `%LocalAppData%` (Windows). Atomicidad probada con tres casos
portables de fallo: padre existente como archivo, destino como directorio (Rename falla en
los tres SO) y directorio de solo lectura (Unix); sin `.tmp` restantes en éxito ni fallo.
Permisos 0600/0700 asertados solo en Unix (decisión 3). `bash scripts/check.sh` verde
(salida 0).

### L3 · 2026-10-28 · ronda de corrección del verificador

Verificador: PASS WITH FIXES (un blocker, un major). Correcciones aplicadas:

- **Blocker (XDG en macOS)**: la precedencia es ahora explícita en las tres rutas
  (`resolveBase`): 1º variable XDG con valor absoluto, 2º respuesta de la plataforma
  (`os.UserConfigDir`/`os.UserCacheDir`; para datos `~/.local/share` en Linux y
  `~/Library/Application Support` en macOS), 3º error si nada da una ruta absoluta.
  `os.UserConfigDir` sólo lee XDG en Linux, así que la variable se comprueba antes.
  Los tests esperan el fallback específico por SO (`runtime.GOOS`) y en macOS no se salta
  nada: con XDG absoluta gana la XDG en todos los no-Windows. Ejecutado en Windows;
  las ramas linux/darwin se compilan con `go vet` cruzado y las aserciones las cubre CI.
- **Major (base relativa)**: si la base resuelta está vacía o no es absoluta, error —
  nunca una ruta relativa al CWD (`XDG_DATA_HOME=""` + `LocalAppData=""` devolvía
  `hipoteca-cli\hipoteca.db` con error nil). Un valor XDG relativo también se rechaza.
- **Minor**: `writeFileAtomic` sincroniza el directorio padre tras el `Rename` (no
  Windows) y documenta sus dos límites: `.tmp` huérfanos tras un crash duro no se
  reclaman, y `Rename` sustituye el symlink, no su destino.
- **Sondas del verificador** como tests permanentes en `probe_test.go`: config vacía
  rechazada, `schema_version = -1` rechazado en Load y Save sin tocar el archivo,
  `Load` en ruta inexistente no crea nada, `Save` sustituye un archivo más largo
  exactamente, y en Windows `Save` falla con el destino abierto sin `FILE_SHARE_DELETE`
  dejando el contenido antiguo intacto y sin `.tmp`. RED observado antes de implementar:
  `TestRelativeXDGRejected` e `TestDataPathEmptyBaseErrors` fallaban contra el código
  anterior reproduciendo el blocker y el major.

`bash scripts/check.sh` verde (exit 0).

### L3 · 2026-10-28 · ronda de corrección

El verificador devolvió PASS WITH FIXES con un blocker (macOS no respeta XDG en la stdlib) y un
major (DataPath cayendo al CWD). Se reescribió la resolución de rutas con precedencia explícita
y se añadió el fsync del directorio. Los tests de rutas ahora se ejecutan también en macOS con
la expectativa propia de la plataforma.
