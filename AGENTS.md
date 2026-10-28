# AGENTS.md · hipoteca-cli

Lee `docs/ARRANQUE.md`, `docs/series.md` y `docs/calculos.md` antes de tu primera tarea.

## Qué es

CLI en Go que lee los tipos de referencia oficiales del Banco de España y calcula
revisiones, cuadros de amortización, amortizaciones anticipadas y escenarios de una
hipoteca. Solo lectura frente a terceros.

## Cómo trabajar

- ODD (gentle-ai / gentle-shell): Authorize, Explore, Resolve uncertainty, Classify, Track,
  Implement, Close.
- Una tarea por vez por su ID. Si es grande, `odd/tasks/<id>-<slug>.md` antes de escribir
  código.
- Test-first.
- `internal/finance` es puro: sin red, sin E/S, sin reloj. Todo cambio ahí es de riesgo
  alto.
- Solo `internal/bde` hace HTTP.
- `internal/model` y `schemas/` son el contrato: cambios en PR aparte con `schema_version`.
- Antes de cerrar: `make check` en verde y review gate con informe en
  `odd/reviews/<id>.md`. Una ronda de correcciones.
- Rama `hc/<id>-descripcion`. Commit y PR. No hagas merge a `main`.
- El merge a `dev` usa `--rebase`, así que **reescribe los ids**: los informes de review
  citan el rango revisado y el PR, y la correspondencia con `dev` se anota al cerrar la
  tarea. No abras un PR sólo para corregir hashes.

## Reglas duras

- Nada de float64 para dinero ni tipos. Decimal exacto.
- Nunca inventes un valor de índice. Si no está publicado, `index_not_published`. Los
  escenarios solo con `--assume` y marcados como `assumed`.
- Toda salida con cálculos incluye sus entradas y la atribución exacta del Banco de España
  con fecha de actualización.
- Nunca supongas datos de la escritura (mes de referencia, redondeo, suelo, comisión): si
  faltan, `loan_incomplete`.
- Ningún texto recomienda una decisión financiera.
- Los datos del préstamo nunca salen de la máquina salvo `--with-impact` activado por el
  usuario.
- Nunca modifiques un caso de referencia de `testdata/finance/` para que pase un test.
- Ningún test de PR toca la red.

## Cuándo parar y preguntar

- Un caso de referencia no coincide con el cálculo.
- La API real no coincide con `docs/series.md`, o una serie desaparece.
- Una convención de cálculo no está en `docs/calculos.md`.
- Necesitas cambiar el esquema o un código de salida.
- Aparece un cambio de alcance.
- La review propone un refactor grande (va a REF).

## Comandos

- `make build` · `make check` · `make test` · `make golden` · `HIPOTECA_LIVE=1 go test -tags live ./...`
