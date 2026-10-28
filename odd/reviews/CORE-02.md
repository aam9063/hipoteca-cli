# Review gate · CORE-02: config, rutas XDG y escrituras atómicas

## Cambio bajo revisión

- **Tarea:** CORE-02 — `odd/tasks/CORE-02-config.md`.
- **Rama:** `hc/core-02-config` (base `dev`).
- **Riesgo declarado:** alto, ítem 1 (pérdida de datos que el usuario no puede
  reconstruir).
- **Verificador independiente:** sí. Ítem 1 escalado y aceptado por el plan
  (`independentVerifier: true`); subagente de contexto limpio y sólo lectura.

## Lo que sostiene la durabilidad

`writeFileAtomic`: temporal en el **mismo directorio** (si no, `Rename` no es atómico entre
sistemas de archivos), `Chmod` **antes** de escribir (no hay ventana con datos del usuario
bajo permisos amplios), `Sync` del archivo, `Rename`, y `Sync` del **directorio** después
(cierra el hueco de "corte de luz justo tras el rename"). El `defer` que borra el temporal se
anula sólo después de un rename exitoso.

**Garantía precisa, según la evidencia del verificador:** el destino **nunca queda truncado
ni mezclado**; en todo instante contiene el archivo viejo completo o el nuevo completo. No es
"nunca se pierde" en el sentido de durabilidad ante corte de energía, pero en el peor caso se
conserva la versión anterior.

## El FAIL del verificador

**Veredicto inicial: PASS WITH FIXES**, con un blocker y un major reales.

| Sev. | Hallazgo | Corrección |
|---|---|---|
| **blocker** | `os.UserConfigDir` y `os.UserCacheDir` **sólo respetan XDG en Linux**. En macOS devuelven `~/Library/...`, así que tres tests de rutas habrían fallado en el CI de macOS. La tarea declaraba "probado en los tres SO" sin serlo. | Precedencia explícita en un único `resolveBase`: XDG **absoluto** gana en toda plataforma; si no, la respuesta propia de cada SO; si no hay base absoluta, error. Los tests ya no se saltan en macOS: la expectativa de fallback cambia por `runtime.GOOS`. |
| **major** | `DataPath()` con `LocalAppData` vacío devolvía `hipoteca-cli\hipoteca.db` **con error nil**: una ruta relativa al directorio actual, justo lo que su propio comentario prometía no hacer. | Base vacía o relativa → error. Un `XDG_*` relativo también se rechaza. |
| minor | Sin `fsync` del directorio tras el rename | Añadido en no-Windows. |
| minor | Temps huérfanos de un crash previo no se reclaman; `Rename` reemplaza un symlink en vez de su destino | Documentados como límites en `atomic.go`, no corregidos (reclamar temps ajenos puede borrar el trabajo de otro proceso). |
| nit | El test de directorio de sólo lectura no aplica como root | Aceptado; sólo afectaría a un CI que corra como root. |

Los seis probes del verificador quedaron como `probe_test.go` permanente.

## Verificación del autor sobre la corrección

- `TestDataPathEmptyBaseErrors`, `TestRelativeXDGRejected` y
  `TestSaveFailsWhenDestOpenWithoutShareDelete` (destino abierto sin `FILE_SHARE_DELETE`:
  `Save` falla, contenido viejo byte a byte, sin `.tmp`) → **PASS** en Windows.
- `GOOS=darwin go vet` y `GOOS=linux go vet` → limpios: las ramas que no puedo ejecutar acá
  compilan.
- `bash scripts/check.sh` → **EXIT=0**.
- `go test ./... -count=1` → ok.

## Límite declarado

**En Windows no puedo ejecutar** las ramas de macOS ni de Linux: los tests de rutas con XDG,
el fallback `~/Library/...`, los permisos `0600`/`0700` y el `fsync` del directorio. Ahí las
verifiqué por lectura y por compilación cruzada. **La prueba real es el CI de macOS y Ubuntu**;
es exactamente la razón por la que la matriz de tres sistemas existe, y esta vez el CI no es
una formalidad: es la única ejecución posible de la mitad de la tarea.

## Veredicto

**PASS tras la ronda única de correcciones.** El blocker y el major están corregidos; la
durabilidad del destino la sostuvo el verificador con probes adversariales, incluido el caso
de destino bloqueado en Windows.
