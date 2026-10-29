# Review gate · CMD-01: comandos `index`, `calc`, `revision` y `schedule`

## Cambio bajo revisión

- **Tarea:** CMD-01 — `odd/tasks/CMD-01-comandos.md`.
- **Rama:** `hc/cmd-01-comandos` (base `dev`).
- **Riesgo declarado:** ítem 3, contrato de salida nuevo y consumo de red.
- **Verificador independiente:** sí. Plan con `independentVerifier: true`; subagente de
  contexto limpio y sólo lectura, con el binario compilado y captura de bytes.

## Veredicto del verificador: PASS (0 blockers, 0 majors, 3 minors, 4 nits)

Lo que confirmó sobre bytes reales, no sobre lectura de código:

- **Un JSON por comando**: cada éxito emite exactamente un valor más un `\n`, verificado con
  `raw_decode`; cada fallo deja stdout en **0 bytes**.
- **Decimales como cadena con su escala**: `calc --principal 150000.50 --rate 3.100` ecoa
  `"150000.50"` y `"3.100"`. `"3.247"`, `"719.14"`, `"500.00"`, `"0.00"` conservan su texto.
  Ningún camino filtra un decimal como número.
- **Atribución byte-exacta** contra el arranque §6.2, con `not_endorsed: true`.
- **Rangos**: `30M` → 31 puntos, `2025` → 12, `2026` → 9, `MAX` → 333 desde 1999-01; los no
  soportados (`36M`, `12M`, `1989`, años futuros, `ayer`, vacío, `max`) → `usage`/2 con el
  `hint` enumerando los válidos. Un año sin datos → éxito con `observations: []`, que es lo
  que hace la API.
- **Códigos**: 0, 2 (`usage` y `loan_incomplete`), 4 (`not_found`) y 6 (`network`), con el
  sobre exacto y `hint` omitido cuando está vacío.
- **Números**: caso 1 y caso 2 al céntimo contra los JSON de referencia, cruzados además con
  una implementación Python independiente; los 31 puntos de `30M` y los 12 de `2025`
  coincidentes punto por punto con los fixtures del Banco.

## Los tres minors y su corrección

| Minor | Corrección |
|---|---|
| **La guarda anti-decimal se evadía** con alias de import (`shdec.Decimal`), `[]decimal.Decimal` y `map[string]decimal.Decimal` — el verificador lo demostró plantando el caso | Resuelve el paquete por los imports del archivo y desenvuelve `StarExpr`/`ArrayType`/`MapType`; 10 casos de prueba en `app` y en `cli` |
| **`calc` fechaba con el `verified_at` del catálogo** aunque no usa ni un dato del Banco: una fecha que envejece y sugiere una procedencia inexistente | Fecha de elaboración en UTC, inyectable; test que normaliza un instante UTC−2 de las 23:30 al día siguiente |
| **`User-Agent` siempre `hipoteca-cli/dev`**, con un comentario en `bde` prometiendo que CMD-01 lo enchufaría | `bde.SetUserAgent`, `app.NewRuntime(version)`, `cli.Deps.Version`; header capturado por `httptest` y test de wiring que prueba que la versión llega al cliente y no el default |

Nits corregidos: el typo `"indícalos los tres"` y la frase de `docs/series.md` §2.3 que
afirmaba un recorte en cliente que el código no hace (rechaza, no recorta).

## Verificación del autor

- `bash scripts/check.sh` → **EXIT=0**, «No vulnerabilities found».
- `TestUserAgentLlevaLaVersionInyectada`, `TestVersionInyectadaAlcanzaElRuntime` y
  `TestLaVersionInyectadaLlegaAlCliente` → PASS.
- Binario real contra la API del Banco: `index latest euribor-1y` → `3.247` (2026-09) con
  atribución de datos sin procesar; `calc` → 719.14 / 65743.56 / 215743.56 y 500.00 / 0.00 /
  120000.00; `revision` → exit 2, stdout vacío, sobre `loan_incomplete` con hint a `loan add`;
  `--range 36M` → `usage`/2 enumerando los válidos.

## Límites que quedan escritos

- **La guarda anti-decimal no es completa**: no atrapa dot-imports, ni un tipo con nombre que
  envuelva `decimal.Decimal` declarado en otro paquete, ni alias declarados fuera del archivo.
  Está documentado en el Log del documento de tarea. Una guarda con sus puntos ciegos
  escritos es honesta; una que finge ser total, no.
- `revision` y `schedule` existen y fallan con `loan_incomplete` porque los préstamos son
  LOAN-01: un error que dice qué ejecutar, no una ausencia.
- Un `--range` repetido deja ganar al último (comportamiento de cobra); un año sin datos
  devuelve lista vacía, no error.
- H-03 (recibos reales) sigue abierto: es lo único que valida las convenciones contra un
  banco.

## Veredicto

**PASS tras la ronda única de correcciones.** El contrato de salida está verificado sobre
bytes reales y el proyecto ya responde preguntas con datos del Banco de España.
