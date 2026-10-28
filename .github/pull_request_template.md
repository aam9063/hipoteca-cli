<!-- Si el cambio lleva ID de tarea (p. ej. OPS-02), el título debe empezar por ese ID. -->

## Qué cambia

<!-- Resumen en una o dos frases: qué problema resuelve y por qué ahora. -->

## Cómo se ha verificado

- [ ] `make check` en verde
- [ ] Los casos de referencia de `testdata/finance/` pasan sin modificarlos
- [ ] Informe de review en `odd/reviews/<ID>.md` (solo tareas con ID)

## Reglas duras (AGENTS.md)

- [ ] Nada de `float64`/`float32` para dinero ni tipos: decimal exacto
- [ ] Nunca se inventa un valor de índice; los escenarios solo con `--assume` y marcados como `assumed`
- [ ] Toda salida con cálculos incluye sus entradas y la atribución exacta del Banco de España con fecha de actualización
- [ ] Ningún test de PR toca la red
