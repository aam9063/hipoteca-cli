# CMD-01: comandos `index`, `revision`, `schedule` y `calc`

## Objetivo

Convertir la CLI en algo usable: que un agente (o una persona) pueda pedir el Euríbor de un
mes, la revisión de su cuota, el cuadro de amortización y un cálculo suelto, con la salida
JSON del contrato de CORE-01.

## Problema y motivo

Todo lo construido hasta aquí es librería. Sin estos comandos el proyecto no responde
ninguna de las preguntas del arranque §1. Es también donde el contrato de salida se estrena
de verdad y donde aparecen las decisiones de forma (qué campos, qué nombres, cómo se declara
una entrada).

## Alcance

- Dentro:
  - `hipoteca index list` — índices del catálogo con su último valor y fecha.
  - `hipoteca index show <id> --range 30M|60M|MAX|<año>` — histórico mensual.
  - `hipoteca index latest <id>...` — último valor.
  - `hipoteca calc --principal --rate --months` — cálculo suelto, sin préstamo guardado.
  - `hipoteca revision` y `hipoteca schedule` — **sólo** si hay un préstamo, y eso es
    LOAN-01; aquí se dejan declarados y respondiendo `loan_incomplete` con el `hint`
    correcto, nunca con datos inventados.
  - Structs de salida con `model.Decimal`, `schema_version` y la atribución de P-05.
- Fuera:
  - Guardar préstamos (`loan add/show/...`) y la config TOML de préstamos: LOAN-01.
  - `amortize`, `scenario`, `compare`: FIN-03 y FIN-04.
  - `--pretty`: se puede añadir después sin tocar el contrato.

## Decisiones

1. **Los structs de salida llevan `model.Decimal`**, nunca `decimal.Decimal` (CORE-01). La
   guarda de AST de `internal/model` se **extiende a los paquetes que crean salida**
   (`internal/app` y `internal/cli`), que es el pendiente declarado al cerrar CORE-01.
2. **Cada respuesta incluye `schema_version`, sus entradas y el bloque `attribution`**:
   datos sin procesar para `index *`, datos procesados para `calc`. `not_endorsed` siempre.
3. **`revision` y `schedule` sin préstamo devuelven `loan_incomplete`** (código 2) con el
   `hint` de `loan add`, porque el préstamo es LOAN-01. Es preferible un error honesto a un
   comando que finge funciona.
4. **`--range` se traduce al rango del contrato**: `30M`, `60M`, `MAX` se pasan tal cual; un
   año con datos se pasa como año; cualquier otra cosa es `usage`/2 con la lista de valores
   aceptados. Nada de recortes silenciosos.
5. **Ningún texto recomienda**: `scenario` y `compare` no entran aquí, y `calc` sólo devuelve
   cifras.
6. **`index` usa la caché** y respeta `--no-cache` si se añade; sin red y sin caché → código 6.

## Superficie de edición (nuestra)

- `internal/app/**`, `internal/cli/**`, `internal/model/**` (sólo si hace falta un tipo común)
- `odd/tasks/CMD-01-comandos.md`, `odd/reviews/CMD-01.md`

## Riesgo (nuestra)

**Medio-alto.** No hay dinero nuevo (el cálculo ya está validado), pero sí contrato de salida
nuevo y consumo de la red: un campo mal nombrado o una atribución ausente se queda para
siempre. Ítem 3.

## Checklist

- [x] CMD-01.1 `index latest` y `index list` con caché · Criterio: golden sobre `httptest`
      con los fixtures reales; atribución de datos sin procesar.
- [x] CMD-01.2 `index show` con los cuatro rangos válidos y error de uso para el resto.
- [x] CMD-01.3 `calc` · Criterio: reproduce el caso 1 y el caso 2 de referencia al céntimo.
- [x] CMD-01.4 `revision` y `schedule` sin préstamo → `loan_incomplete`/2 con `hint`.
- [x] CMD-01.5 Guarda de AST extendida a `internal/app` y `internal/cli`: `model.Decimal`
      obligatorio en structs de salida.
- [x] CMD-01.6 e2e: stdout con un único JSON, stdin/red simulada, códigos 0, 2 y 6.
- [ ] CMD-01.7 Verificador independiente · Criterio: informe en `odd/reviews/CMD-01.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Implementadas las capas `internal/app` (orquestación, sin cobra) e `internal/cli`
(comandos finos), con la guarda de AST extendida y los tests de app, e2e y guarda en
verde. Pendiente CMD-01.7 (verificador independiente, informes en `odd/reviews/`).

### Decisiones tomadas al implementar

- `app` consume el Banco por una interfaz (`app.Source`: `Latest`/`History`) que
  `*bde.HTTPClient` satisface por construcción: es la única costura de prueba, porque
  `bde.HTTPClient` no expone la base URL fuera del paquete y el catálogo no acepta
  overrides de endpoint. `app.NewRuntime()` hace el cableado de producción (config →
  catálogo con overrides → cliente con caché de `config.CacheDir()`; un fallo de
  directorio de caché degrada a sin caché, nunca bloquea).
- `LatestIndices`/`IndexList` reciben también el catálogo porque la salida devuelve el
  `id` y el `name` de `series.yaml` (la respuesta de `favoritas` no los trae).
- Atribución: `index *` usa la de datos sin procesar fechada al mes más reciente de la
  fuente (`fechaValor`/`fechaFin`, su día 1); `calc` usa la de datos procesados fechada
  al `verified_at` del catálogo (2026-10-28), la única fecha real que el binario asocia
  a los datos originales cuando no hay red. Constante `catalogVerifiedAt` en `app` con
  un test que la compara contra `series.yaml` para que no se desvíe.
- Los rangos de `--range` se validan en la CLI (`parseRange`): 30M/60M/MAX tal cual y
  años 1990..actual; el resto usage/2 con la lista de valores aceptados.

## Siguiente paso

Verificación independiente (CMD-01.7): informe en `odd/reviews/CMD-01.md`.

## Log

### L1 · 2026-10-28 · decisión del usuario

"Seguimos". Tras cerrar FIN-02 y quedar bloqueado el review nativo del candidato acumulado
(por presupuesto de contexto del proveedor), se continúa con CMD-01, la primera tarea que
hace visible todo lo anterior.

### L2 · 2026-10-28 · implementación

Implementación test-first de CMD-01.1–1.6: rojo observado (compilación), verde en
`internal/app` y `internal/cli`, guarda de AST probada con una violación temporal en
ambos paquetes. Suite completa y `bash scripts/check.sh` en verde. CMD-01.7 pendiente.

### L3 · 2026-10-28 · ronda de corrección tras el verificador independiente

Tres menores y dos nits del informe (PASS) corregidos test-first:

- **Guarda anti-decimal**: resuelve el paquete del selector a través de las
  importaciones del fichero (un alias `shdec.Decimal` ya no pasa) y desenvuelve
  `*T`, `[]T` y `map[K]T` (`[]decimal.Decimal`, `map[string]decimal.Decimal`).
  Casos de evasión añadidos como tests en `internal/app/nofloat_test.go` y
  `internal/cli/nofloat_test.go` (rojo observado contra la guarda antigua).
  **Límites residuales conocidos de la guarda, a propósito aquí para que nadie
  la crea completa**: (1) un dot-import (`import . "github.com/shopspring/decimal"`)
  deja `Decimal` como identificador pelado, sin selector, y no se ve; (2) un
  tipo con nombre definido en *otro* paquete que envuelva a shopspring
  (`type T decimal.Decimal`) usado como campo (`T`) pasa; (3) un alias de tipo
  declarado en otro paquete (`type D = decimal.Decimal`) usado como campo
  (`D`) pasa. Dentro del propio paquete sí cubre todo struct de los ficheros
  no-test, incluidas las variantes anteriores si el alias/tipo se declara ahí.
- **Atribución de `calc`**: deja de usar `catalogVerifiedAt` (`verified_at` del
  catálogo, que no era procedencia de nada: `calc` no consume datos del Banco).
  `app.Calc` recibe `now time.Time` y la atribución de datos procesados lleva
  como `data_updated_at` la **fecha de elaboración**: el día de la ejecución,
  en UTC (`YYYY-MM-DD`). Esto **sustituye** la decisión anotada en L2 sobre la
  atribución de `calc`; el test que ataba la constante a `series.yaml` se
  elimina y no debe recrearse.
- **Nits**: errata `"indícalos los tres"` → `"indica los tres"` en `calc`; y
  docs/series.md §2.3 ahora dice que la CLI rechaza deliberadamente (usage/2)
  los rangos que la API no admite (`36M`, `12M`, …) en vez de recortarlos en
  cliente — el ejemplo `--range 36M` del arranque está sin ofrecer a propósito.
- **Pendiente**: el User-Agent real (`hipoteca-cli/<version>`) no se pudo
  enchufar en esta ronda: el cableado del cliente pasa por
  `app.NewRuntime()` (`internal/app/index.go`) y `Deps.runtime()`
  (`internal/cli/root.go`), fuera de las superficies de edición de la ronda.

### L4 · 2026-10-28 · cierre del gate

Verificador independiente: **PASS** con 0 blockers y 0 majors, sobre 1.840 líneas. Tres minors
y cuatro nits, todos corregidos en una sola ronda:

- la guarda anti-decimal se evadía con alias de import, slices y maps; ahora resuelve el
  paquete por los imports y desenvuelve `ArrayType`/`MapType` (10 casos de prueba);
- `calc` fechaba su atribución con el `verified_at` del catálogo aunque no usa datos del
  Banco; ahora usa la fecha de elaboración en UTC;
- el `User-Agent` era siempre `hipoteca-cli/dev` y había un comentario prometiendo que
  CMD-01 lo enchufaría; ahora la versión inyectada llega al cliente.

Límites residuales de la guarda, escritos a propósito: no atrapa dot-imports, ni un tipo con
nombre que envuelva `decimal.Decimal` declarado en otro paquete, ni alias declarados fuera.
