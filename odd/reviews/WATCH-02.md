# WATCH-02 · El aviso como servicio

## Qué se hizo

`hipoteca watch service install|uninstall|status`: deja `watch run` corriendo como servicio
del usuario (systemd en Linux, LaunchAgent en macOS), para que el aviso no muera al cerrar la
terminal.

## Las decisiones que definen la tarea

**La unidad usa la ruta absoluta del ejecutable**, no `hipoteca` a secas. Un servicio no
hereda el PATH del usuario: con la forma corta el aviso funcionaría hasta el primer reinicio y
después moriría en silencio, sin que nadie se entere de que dejó de avisar. `ServiceUnitFor`
lo exige y por eso es imposible generar una unidad rota por descuido.

**La unidad fija `XDG_DATA_HOME`.** Por la misma razón: el servicio debe encontrar la misma
base de datos que vio el usuario al configurar las vigilancias, no la que le toque por el
entorno del arranque.

**`uninstall` desactiva antes de borrar.** En systemd el estado `enabled` vive fuera del
archivo de la unidad y sobrevive a borrarlo: quedan restos, y el criterio de aceptación de la
tarea es justo «sin restos». En launchd el agente se descarga antes de que desaparezca el
plist.

**Windows es `usage`, no error interno.** El mensaje dice qué pasa y manda a
`watch run` como tarea programada. No es que el programa se rompa: es que ese sistema no
ofrece dónde engancharlo. Igual criterio para `install` sin vigilancias: un servicio que no
vigila nada es una trampa silenciosa, así que se rechaza antes de escribir nada.

## Un defecto que sólo aparece cruzando sistemas

La primera versión validaba la ruta con `filepath.IsAbs` y construía con `filepath.Join`. Los
tests fallaron en Windows con `el ejecutable debe ser una ruta absoluta, no
"/opt/hipoteca/bin/hipoteca"`: **`/opt/...` no es una ruta absoluta en Windows**, aunque la
unidad que se está generando sea para Linux.

La generación tiene que usar la semántica del sistema **destino**, no la del que compila:
ahora valida con prefijo `/` y compone con `path.Join`. Y el rechazo por sistema no soportado
se decide **antes** de mirar rutas, para que Windows diga «no soportado» en lugar de quejarse
de una ruta que en Windows no significa lo mismo.

Los tests de escritura no usan `ServiceUnitFor`: arman la unidad sobre el sistema de archivos
real del test. Así un test verifica «qué contenido toca» y el otro «dónde vive este archivo
aquí», y ninguno depende de en qué sistema se ejecute.

## Verificación

- `go test ./internal/watch/ -run Service` → **7 de 7 en verde**: ruta absoluta en systemd y
  en el plist, tres entradas que romperían el servicio (ruta relativa, intervalo menor de una
  hora, Windows), idempotencia sin duplicar unidades, `uninstall` sin restos de archivo ni
  directorio, `uninstall` dos veces, y `status` antes y después.
- `bash scripts/check.sh` → **EXIT=0**.
- Con el binario: `watch service status` en Windows → exit **2** con «este sistema (windows) no
  tiene un servicio de usuario soportado»; `watch service install` sin vigilancias → exit
  **2** con «no hay ninguna vigilancia que el servicio pueda atender».

## Límite

El gestor de servicios real **no se ejercita en los tests**: se inyecta un runner falso. Es
deliberado —hablar con systemd o launchd no es determinista y no debe pasar en CI—, pero
significa que la integración con el gestor sólo se prueba de verdad al usarla en una máquina
Linux o macOS. El contenido de la unidad y los comandos que se le pasan al gestor sí están
fijados por test.

## Corte

Por decisión del usuario (`odd/tasks/WATCH-02-servicio.md`, L3), los avisos van a `v0.2.0`:
esta tarea se entrega a `dev` pero **no entra en `v0.1.0`**.
