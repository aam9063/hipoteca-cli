# AGENT-01 · Que un agente resuelva sin contexto

## Qué se hizo

Cuatro comandos, acotados a las Olas 2–3 por la decisión de corte (los avisos van a `v0.2.0`):

- `hipoteca skills list|get` — las cuatro recetas de `docs/ARRANQUE.md` §6.5 como datos
  embebidos. `get` es el único comando que responde texto plano.
- `hipoteca schema` — el contrato de salida de los comandos de las Olas 2–3.
- `hipoteca doctor` — versión y commit, rutas y permisos, catálogo, préstamos con sus campos
  faltantes, y una comprobación de red que `--offline` apaga.
- `hipoteca config show|get|set` — cierra el hueco que WATCH-01 dejó declarado: declarar un
  sink ya no exige editar el TOML a mano.

## El criterio de aceptación, comprobado a mano

No es «existen los comandos», es: **un agente sin contexto resuelve las cuatro preguntas de
§6.5 usando sólo `skills get`**. Leí `next-revision` entera y contiene lo que hace falta:

- El comando exacto con todos sus flags, incluidos los que son datos de la escritura
  (`--reference-offset-months`, `--floor none`, `--rate-rounding`), explicados uno por uno.
- Qué hacer cuando el préstamo está incompleto: `loan show` lista `missing_fields`, y se
  arregla con `loan edit` — la recuperación concreta, no «consulta la documentación».
- Qué hacer con `index_not_published`: esperar al Banco, o `--assume` si **el usuario** da el
  valor, con la advertencia de que la salida lo marcará `assumed`.
- Cómo leer la salida (`index`, `rate`, `payment`, `inputs.outstanding_source`) y que los
  importes son cadenas decimales exactas.
- La atribución literal al Banco de España y la prohibición de recomendar.

## Las decisiones que definen la tarea

**`schema` no puede desfasarse.** Se describe a partir de los **mismos structs** que los
comandos escriben, no de una tabla paralela, y los códigos de salida salen de la tabla de
`internal/errs`. Un test compara el schema de `calc` con la salida real del comando: si el
walker de reflexión y `encoding/json` discreparan (embedded, `omitempty`), falla.

**Ningún secreto en la salida.** Comprobado con un sink que lleva credenciales: `config set`
responde `https://redacted@ntfy.sh/...`, `config show` lo mismo, y `grep` del secreto en toda
la salida da **0 apariciones**; en disco queda la URL real, que es donde tiene que estar.

**Un sink incompleto no falla en silencio.** Con `url` pero sin `type`, `watch add` lo rechaza
con `usage`/2 y un mensaje que nombra el tipo vacío. El hint se corrigió en esta tarea: decía
«cambia el type del sink en la configuración» cuando ahora existe el comando que lo hace;
ahora dice `hipoteca config set sinks.<nombre>.type webhook`.

## Verificación

- `bash scripts/check.sh` → **EXIT=0**.
- `go test ./internal/skills/ ./internal/cli/ ./internal/app/` → verde.
- Con el binario: `skills list` (un JSON), `skills get next-revision` (texto), `skills get
  nope` (exit 4 con los ids válidos en el hint), `schema calc` (un JSON), `config set`
  con credenciales (redactado), `config set loans.*` (exit 2 con hint a `loan edit`),
  `doctor --offline` (`network.checked: false`).

## Dos incidentes del camino

**El worker escribió en el `%AppData%` real del usuario** durante una demo, porque en Windows
`internal/config` ignora `XDG_*` y usa `%AppData%`. Lo restauró, y verifiqué que el archivo
quedó válido y vacío (38 bytes, `schema_version = 1`, `[indices]` y `[sinks]` sin entradas).
Limpié también el scratch que dejó fuera del worktree. Es una lección de método: **un worker
que demuestra comandos necesita un HOME aislado, y en Windows eso significa `APPDATA`.**

## Límite declarado

El test de deriva compara el schema de `calc` con su salida real. **Los demás comandos no
tienen esa comprobación de deriva**: comparten el walker, así que un error de forma en el
walker se vería en todos, pero un struct con un caso raro (un puntero, un `interface{}`)
podría describirse mal sin que ningún test lo note. Es aceptable para v0.1.0 y hay que
vigilarlo si el contrato crece.

`doctor` comprueba la red con un **dial TCP al host del catálogo**, no con una petición HTTP:
respeta la regla de que sólo `internal/bde` habla con el Banco, y sólo dice «hay red», que es
lo que `doctor` necesita.
