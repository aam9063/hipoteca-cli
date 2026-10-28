# hipoteca-cli: documento de arranque

Versión 0.1 · 28 de octubre de 2026 · Estado: diseño. Ningún código de este documento se ha ejecutado todavía.

> Este documento es la fuente de verdad para crear el repositorio. Lo leen tus agentes en gentle-shell (Pi) con ODD. Si algo de aquí contradice a `AGENTS.md`, manda `AGENTS.md` y se corrige este documento en un PR.

---

## 0. Cómo usar este documento

1. Haz la Ola 0 (sección 14): instalar gentle-shell, proteger `main` y el SPIKE-01 (localizar los códigos de serie en el Banco de España).
2. Crea el repo `aam9063/hipoteca-cli` y copia este archivo como `docs/ARRANQUE.md`.
3. Abre gentle-shell en la carpeta del repo y pega el prompt de la sección 16.
4. Autoriza ola por ola. Los agentes abren PR; el merge lo haces tú.

---

## 1. Qué es y para qué sirve

**hipoteca-cli** es una herramienta de línea de comandos open source (MIT) que lee los **tipos de referencia oficiales del mercado hipotecario** que publica el Banco de España (Euríbor, IRPH y otros) y hace los cálculos de tu hipoteca: revisión de cuota, cuadro de amortización, amortización anticipada y escenarios. Todo en JSON para que un agente de IA lo use.

Ejemplos de uso real con un agente:

- "¿Cuánto me va a cambiar la cuota en la revisión de enero?"
- "Avísame cuando se publique el Euríbor de este mes."
- "Si amortizo 10.000 €, ¿me conviene reducir cuota o plazo? ¿Cuántos intereses me ahorro?"
- "Mi hipoteca es IRPH: ¿qué habría pagado en los últimos 5 años con Euríbor más 1 %?"
- "¿Qué cuota tendría si el Euríbor sube al 3 % o baja al 1,5 %?"
- "Evolución del Euríbor en los últimos 3 años."

### 1.1 Lo que NO hace (no objetivos)

- **No es asesoramiento financiero.** Calcula y compara; no recomienda productos ni decisiones. Se dice en el README, en `doctor` y en la skill.
- **No sustituye a tu banco.** El cálculo exacto de tu banco puede diferir en céntimos por convenciones (base de cálculo de días, redondeos). La CLI muestra siempre las entradas y la fórmula para que se pueda contrastar, y permite resincronizar con el capital pendiente real de tu recibo.
- **No se conecta a tu banco** ni pide credenciales de nada.
- **No envía tus datos** de hipoteca a ningún sitio.
- **No lleva un LLM dentro.** Matemática financiera determinista.

### 1.2 Por qué es distinta de lo que ya existe

Hay simuladores web de bancos y portales, y un paquete de R (tidyBdE) para descargar estadísticas del Banco de España. No hay ninguna CLI ni servidor MCP que combine **los índices oficiales** con **los datos de tu hipoteca** y los cálculos de revisión y amortización, pensada para agentes.

---

## 2. La fuente de datos (confirmado)

### 2.1 Qué publica el Banco de España

Según su portal del cliente bancario, los tipos de referencia oficiales del mercado hipotecario son:

| Índice | Nota |
|---|---|
| **Euríbor a un año** | El más usado en revisiones de hipotecas variables. Valor mensual = media simple de los valores diarios del mes |
| Euríbor a una semana, un mes, tres meses y seis meses | Medias mensuales (calculadas por EMMI) |
| **IRPH** (conjunto de entidades de crédito) | Préstamos para vivienda libre a tres años o más |
| IRS (permuta de intereses) a cinco años | Media mensual |
| Rendimiento interno de la deuda pública (2 a 6 años) | |
| Tipo medio de préstamos hipotecarios a 1 a 5 años (zona euro) | |
| Tipo basado en el €STR | Varios plazos |

Base normativa citada por el propio Banco: artículo 27 de la Orden EHA/2899/2011 y norma 14 y anejo 8 de la Circular 5/2012 (con sus modificaciones).

### 2.2 La API

| Dato | Estado |
|---|---|
| Base | `https://app.bde.es/bierest/resources/srdatosapp/` |
| Último valor | `favoritas?idioma=es&series=<códigos separados por comas>` |
| Histórico | `listaSeries?idioma=es&series=<códigos>&rango=<rango>` |
| Rangos | Un año (`2024`) o `30M`, `60M`, `MAX` para series mensuales |
| Formato | JSON. Fechas `yyyy-mm-ddThh:mm:ssZ` (series mensuales con día 1) |
| Campos | `serie`, `descripcionCorta`, `codFrecuencia`, `decimales`, `simbolo`, `fechaValor`, `valor`; en histórico `fechas` y `valores` en el mismo orden |
| Autenticación | No documentada (ningún token en los ejemplos oficiales) |
| Codificación | En los códigos, `#` va como `%23` y `%` como `%25` |
| Ejemplos oficiales | Notebook de Python y R Markdown en la web del Banco |
| Límites | **No documentados.** No se garantiza servicio ininterrumpido |

**Pendiente (SPIKE-01, tarea de localización, no de viabilidad):** los **códigos de serie** exactos de cada índice. Se buscan con la aplicación HERMES del Banco de España y se fijan en `internal/bde/series.yaml`.

### 2.3 Condiciones de reutilización (confirmado)

De sus términos de uso de estadísticas:

- Acceso y reutilización **libres y gratuitos**, sin distinguir uso comercial.
- **Citar siempre:**
  - Datos sin tocar: `Fuente: Sitio web del Banco de España (www.bde.es)`.
  - Datos procesados (todos nuestros cálculos): `Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)`.
  - Indicar la **fecha de la última actualización** cuando el original la tenga.
- **No alterar** el contenido ni los metadatos, ni cambiar su sentido.
- **No insinuar** que el Banco de España participa, patrocina o respalda la herramienta.
- El Banco puede cambiar, suspender o retirar series y formatos.
- Si alguien vendiera material con estos datos, tendría que indicar que son gratuitos en la web del Banco.

Todo esto se traduce en el bloque `attribution` (sección 6.2) y en P-05.

---

## 3. Principios no negociables

| ID | Principio |
|---|---|
| P-01 | **JSON por defecto** en stdout, un único valor JSON por comando (o JSONL). Diagnóstico solo en stderr. |
| P-02 | **Códigos de salida estables** y sobre de error JSON (sección 6.4). |
| P-03 | **Tus datos no salen de tu máquina.** La hipoteca vive en config local `0600`. La única red es `app.bde.es` (y los sinks que configures, que por defecto no llevan datos de tu préstamo). |
| P-04 | **Cálculo transparente.** Toda cifra calculada va acompañada de sus entradas: índice y mes usados, diferencial, TIN resultante, capital, plazo, convención y redondeo. |
| P-05 | **Atribución exacta** según los términos del Banco de España, con fecha de actualización. Nunca se sugiere respaldo oficial. |
| P-06 | **Nunca se inventa un índice.** Si el valor de un mes no está publicado, error `index_not_published`. Un escenario hipotético exige `--assume` explícito y la salida lo marca como `assumed`. |
| P-07 | **Dinero en decimal exacto.** Nada de `float64` para importes ni tipos. Redondeo a céntimos solo al final de cada cuota, con regla configurable. |
| P-08 | **No es asesoramiento.** Ningún comando dice "te conviene". `compare` y `amortize --mode compare` devuelven cifras, no recomendaciones. |
| P-09 | **El esquema de salida es un contrato** con `schema_version`. Cambios en PR aparte y en el CHANGELOG. |

---

## 4. Dominio: la hipoteca

### 4.1 Datos del préstamo (config local)

```toml
[loans.casa]
name = "Casa"
type = "variable"              # variable | mixta | fija
start_date = "2021-06-01"      # fecha de la primera cuota o de firma, según escritura
original_principal = "180000.00"
term_months = 360
payment_day = 1

# Tramo inicial (meses a tipo fijo antes de la primera revisión)
initial_rate = "1.25"          # TIN anual % del periodo inicial
initial_period_months = 12

# Revisiones (variable y mixta)
index = "euribor-1y"           # euribor-1y | irph | ...
spread = "0.89"                # diferencial en puntos
spread_unbonified = "1.39"     # opcional: diferencial sin bonificaciones
revision_every_months = 12
reference_offset_months = 2    # qué mes de índice se usa: N meses antes del mes de revisión, según escritura
floor_rate = ""                # suelo, si lo hubiera (vacío = sin suelo)
rate_rounding = "none"         # none | 1/8 | 1/4 | 0.05 ... según escritura

# Convenciones de cálculo
interest_convention = "monthly-nominal"  # TIN/12 por mes (estándar sistema francés)
payment_rounding = "half-up-cents"

# Amortización anticipada
early_repayment_fee_pct = "0"  # comisión que figure en tu escritura

# Resincronización con el banco (opcional, recomendado)
[loans.casa.sync]
date = "2026-10-01"
outstanding_principal = "152341.18"   # capital pendiente según tu último recibo
current_rate = "3.12"
```

- Todos los campos que dependen de **tu escritura** (`reference_offset_months`, `rate_rounding`, `floor_rate`, `early_repayment_fee_pct`) son **obligatorios o explícitos**: la CLI no los supone. `loan add` los pregunta y `loan show` avisa si faltan.
- `sync` permite partir del capital pendiente real y no arrastrar diferencias de céntimos de años de cálculo.

### 4.2 Cálculos (sistema francés)

- Tipo mensual: `i = TIN / 12 / 100`.
- Cuota: `C = P · i / (1 − (1 + i)^−n)`, con `P` el capital pendiente y `n` los meses restantes. Si `i = 0`, `C = P / n`.
- En cada mes: interés `= P · i`, amortización `= C − interés`, `P` nuevo `= P − amortización`.
- **Revisión:** `TIN nuevo = índice(mes de referencia) + diferencial`, aplicando el suelo si lo hay y el redondeo de la escritura. Se recalcula `C` con el capital pendiente y el plazo restante en la fecha de revisión.
- **Mixta:** tramo fijo con `initial_rate` y después revisiones.
- **Amortización anticipada:** comisión `= importe · early_repayment_fee_pct / 100`. Dos modos: `reduce-payment` (mismo plazo, cuota menor) y `reduce-term` (misma cuota, menos meses). `compare` devuelve ambos con intereses restantes totales y ahorro respecto a no amortizar.
- **Comparativa de índices:** recalcula la historia del préstamo con otro índice y otro diferencial (por ejemplo IRPH frente a Euríbor más 1 %) usando los valores oficiales de cada mes.

Toda esta lógica vive en `internal/finance`, sin red y sin E/S, con tests exhaustivos (sección 9).

---

## 5. Diseño de la CLI

### 5.1 Comandos

| Comando | Qué hace |
|---|---|
| `hipoteca index list` | Índices disponibles, código de serie, frecuencia, último valor y su fecha |
| `hipoteca index show <índice> [--range 36M\|MAX\|2025]` | Histórico mensual |
| `hipoteca index latest [<índice>...]` | Último valor publicado |
| `hipoteca loan add <id>` / `edit` / `show` / `list` / `remove` | Préstamos guardados en local |
| `hipoteca loan sync <id> --date ... --outstanding ... --rate ...` | Resincronizar con tu recibo |
| `hipoteca revision [<id>] [--date 2027-01] [--assume 2.35]` | Próxima revisión (o la de una fecha): índice usado, TIN nuevo, cuota nueva, diferencia |
| `hipoteca schedule [<id>] [--from ...] [--to ...]` | Cuadro de amortización (JSONL recomendado) |
| `hipoteca amortize [<id>] --amount 10000 --date 2027-03 --mode reduce-payment\|reduce-term\|compare` | Amortización anticipada |
| `hipoteca scenario [<id>] --index-values 1.5,2,2.5,3` | Cuota y coste total con varios valores del índice |
| `hipoteca compare [<id>] --index irph --spread 0 --vs euribor-1y --vs-spread 1 [--from 2021-06]` | Qué habrías pagado con otro índice |
| `hipoteca calc --principal 150000 --rate 3.1 --months 300` | Cálculo suelto sin guardar préstamo |
| `hipoteca watch add index <índice> [--notify phone] [--with-impact]` / `list` / `remove` / `check` / `run` | Aviso de nuevo valor publicado |
| `hipoteca watch service install / uninstall / status` | systemd o launchd |
| `hipoteca sink list / test` | Destinos de avisos |
| `hipoteca doctor` | Config, préstamos incompletos, red, series |
| `hipoteca skills list / get` | Guía embebida para agentes |
| `hipoteca schema [comando]` | JSON Schema de cada salida |
| `hipoteca config ...` / `version` / `update` | |

Si solo hay un préstamo guardado, `<id>` es opcional.

### 5.2 Ejemplo: `hipoteca revision casa --date 2027-01`

```json
{
  "schema_version": 1,
  "loan": "casa",
  "revision_date": "2027-01-01",
  "index": {
    "id": "euribor-1y",
    "reference_month": "2026-11",
    "value": "2.211",
    "status": "published",
    "series": "<código BdE>",
    "published_at": "2026-12-01"
  },
  "rate": {
    "previous": "3.120",
    "spread": "0.890",
    "raw": "3.101",
    "after_floor": "3.101",
    "after_rounding": "3.101",
    "new": "3.101"
  },
  "inputs": {
    "outstanding_principal": "149876.42",
    "remaining_months": 293,
    "outstanding_source": "sync+schedule",
    "interest_convention": "monthly-nominal"
  },
  "payment": {
    "previous": "742.18",
    "new": "740.31",
    "difference_month": "-1.87",
    "difference_year": "-22.44"
  },
  "warnings": [],
  "attribution": {
    "text": "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)",
    "data_updated_at": "2026-12-01",
    "not_endorsed": true
  }
}
```

(Valores ilustrativos.)

- Si el mes de referencia aún no está publicado: código 4 `index_not_published` con `hint: "usa --assume <valor> para un escenario"`.
- Con `--assume`, `index.status = "assumed"` y aparece un aviso en `warnings`.

### 5.3 Vigilancias

```
hipoteca watch add index euribor-1y --notify phone
hipoteca watch service install --interval 12h
```

- Evento `index.published`: hay un mes nuevo en la serie.
- El aviso lleva solo el índice, el mes, el valor y la atribución.
- `--with-impact` añade el cambio estimado de **tu** cuota al aviso. Es opcional porque implica enviar un dato de tu préstamo al sink: la CLI lo advierte al activarlo (un topic de ntfy público lo puede leer quien lo adivine).
- Intervalo mínimo: 1 hora. Por defecto: 12 horas. Los índices son mensuales.

---

## 6. Contrato para agentes

### 6.1 Salida

- stdout: un valor JSON (o JSONL). stderr: diagnóstico y errores en JSON si stdout no es terminal.
- **Importes y tipos como cadenas decimales** (`"742.18"`, `"3.101"`), para no perder precisión en el JSON. Meses como enteros. Fechas ISO 8601.
- `pretty` en español con formato es-ES (`742,18 €`, `3,101 %`).

### 6.2 Atribución

- Salidas con datos sin procesar (`index list/show/latest`): `Fuente: Sitio web del Banco de España (www.bde.es)` y `data_updated_at`.
- Salidas con cálculos: `Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)` y `data_updated_at`.
- `not_endorsed: true` y el README dice que el proyecto no está afiliado ni respaldado por el Banco de España.

### 6.3 `schema`

`hipoteca schema revision` imprime el JSON Schema. Los tests validan la salida real contra él.

### 6.4 Errores y códigos de salida

```json
{"error": {"type": "index_not_published", "message": "El Euríbor a un año de 2026-12 aún no está publicado.", "exit_code": 4, "hint": "hipoteca revision casa --date 2027-02 --assume <valor>"}}
```

| Código | Tipos | Significado |
|---|---|---|
| 0 | | Éxito |
| 1 | `internal` | Error inesperado |
| 2 | `usage`, `loan_incomplete`, `cancelled`, `confirmation_required` | Argumentos inválidos, préstamo sin un dato de la escritura, cancelado, falta `--yes` en un borrado |
| 4 | `not_found`, `index_not_published` | Préstamo o índice inexistente; valor del mes aún no publicado |
| 5 | `upstream_unavailable`, `rate_limited` | El Banco de España no responde o limita |
| 6 | `network` | Sin red y sin caché |
| 7 | `api_changed` | Respuesta con forma inesperada o serie retirada. Enlace a issue prellenada |
| 130 | | Interrumpido |

No hay código 3: no hay autenticación.

### 6.5 Skill embebida

```markdown
---
name: hipoteca
description: Consultar Euríbor, IRPH y otros índices oficiales del Banco de España y calcular revisiones, cuadros de amortización, amortizaciones anticipadas y escenarios de una hipoteca con la CLI hipoteca.
---

# hipoteca

## Reglas
- Usa la salida JSON. Explica siempre con qué índice, mes y diferencial se calculó.
- Si index.status es "assumed", dilo: es un escenario, no un dato oficial.
- No digas "te conviene": presenta las cifras de cada opción y deja la decisión al usuario.
- Recuerda que el banco puede diferir en céntimos y que manda la escritura.
- Si loan_incomplete, pide al usuario el dato de su escritura; no lo supongas.
- Cita: "Elaboración propia con datos del Banco de España".

## Recetas
- Próxima revisión: `hipoteca revision`
- Amortizar: `hipoteca amortize --amount <importe> --date <mes> --mode compare`
- Sensibilidad: `hipoteca scenario --index-values 1.5,2,2.5,3`
- IRPH frente a Euríbor: `hipoteca compare --index irph --vs euribor-1y --vs-spread 1`
```

---

## 7. Configuración y almacenamiento

| Qué | Dónde | Permisos |
|---|---|---|
| Config (préstamos, sinks) | `$XDG_CONFIG_HOME/hipoteca-cli/config.toml` | `0600` |
| Base de datos (vigilancias, eventos) | `$XDG_DATA_HOME/hipoteca-cli/hipoteca.db` | `0600` |
| Caché de series | `$XDG_CACHE_HOME/hipoteca-cli/` (24 h para último valor; histórico cerrado sin caducidad) | `0600` |

- Los meses ya publicados no cambian: se guardan sin caducidad y casi nunca hace falta volver a pedir el histórico.
- Los códigos de serie van compilados en el binario y se pueden sobrescribir en config, por si el Banco los cambia.

---

## 8. Arquitectura

### 8.1 Stack

| Pieza | Elección | Motivo |
|---|---|---|
| Lenguaje | **Go 1.25+** | Binario único multiplataforma, como el resto de tus CLIs |
| CLI | `cobra` | |
| Config | `go-toml/v2` | |
| Decimal | `shopspring/decimal` (o `math/big.Rat` si se prefiere sin dependencias) | P-07 |
| SQLite | `modernc.org/sqlite` | Sin CGO |
| Releases | GoReleaser, `install.sh`, Homebrew tap, Scoop | |

### 8.2 Capas

```
cli (cobra) → app (revision, amortize, compare...) → bde (cliente API, caché, series.yaml)
                       ↓
                 finance (puro: sin red, sin E/S, sin reloj)
```

- `internal/finance` es **puro**: recibe datos y devuelve resultados. Es el corazón del proyecto y el de mayor riesgo.
- `internal/bde` es el único paquete que hace HTTP.
- El reloj se inyecta (para tests de fechas de revisión).

### 8.3 Estructura del repositorio

```
hipoteca-cli/
├── AGENTS.md
├── CLAUDE.md                 una línea: @AGENTS.md
├── README.md                 atribución, "no es asesoramiento", "no afiliado al Banco de España"
├── LICENSE                   MIT
├── CHANGELOG.md
├── go.mod                    module github.com/aam9063/hipoteca-cli
├── Makefile
├── .goreleaser.yaml
├── cmd/hipoteca/main.go
├── internal/
│   ├── cli/
│   ├── app/
│   ├── bde/                  cliente, caché, series.yaml
│   ├── finance/              cuota, cuadro, revisión, amortización, comparativas
│   ├── loan/                 modelo y validación del préstamo
│   ├── model/                salidas y esquema
│   ├── store/
│   ├── watch/                vigilancias, eventos y sinks
│   ├── output/
│   ├── errs/
│   ├── config/
│   └── agentdocs/            SKILL.md embebido
├── schemas/
├── testdata/
│   ├── bde/                  respuestas reales de la API (datos públicos, con atribución)
│   └── finance/              casos de referencia calculados de forma independiente
├── docs/
│   ├── ARRANQUE.md
│   ├── series.md             resultado de SPIKE-01
│   ├── calculos.md           fórmulas, convenciones y ejemplos verificados
│   ├── cli-spec.md
│   └── adr/
├── odd/
│   ├── tasks/
│   └── reviews/
├── scripts/install.sh
└── .github/workflows/ci.yml
```

---

## 9. Calidad y tests

| Nivel | Qué prueba | En CI |
|---|---|---|
| Unidad finanzas | Cuota, cuadro, revisión, suelo, redondeos, mixta, amortización en ambos modos, comisión, tipo 0 % | Sí |
| **Casos de referencia** | Resultados de `internal/finance` contra valores calculados de forma independiente (fórmula PAGO de una hoja de cálculo o `numpy-financial`), documentados en `testdata/finance/` con su origen | Sí |
| Propiedades | La suma de amortizaciones iguala el capital; el capital nunca es negativo; la última cuota deja capital 0 (con tolerancia de céntimos explícita); amortizar nunca aumenta los intereses totales | Sí |
| Cliente BdE | Contra respuestas reales guardadas en `testdata/bde/` | Sí |
| Contrato | Salida valida contra `schemas/` y golden files | Sí |
| CLI punta a punta | Binario real con HOME temporal; servidor `httptest`; códigos 4, 6 y 7; préstamo incompleto | Sí |
| En vivo | Llamada real a la API y validación de las series de `series.yaml` | Job nocturno |

Reglas:

- TDD: primero el test en rojo.
- **Ningún `float64` en `internal/finance`.** Un linter (`forbidigo` o test de AST) lo comprueba.
- Ningún test de PR toca la red.
- Los casos de referencia se generan **fuera** del código del proyecto y se versionan con su origen. Un agente no puede "arreglar" un caso de referencia para que pase un test: si discrepa, se para y pregunta.
- `make check` = `gofmt`, `go vet`, `staticcheck`, `govulncheck`, tests.

---

## 10. Flujo de trabajo: ODD en gentle-shell (Pi)

### 10.1 Qué es cada pieza (según su documentación, leída hoy)

- **gentle-ai** configura el agente que ya usas con memoria persistente (Engram), skills y ODD. No instala agentes.
- **gentle-shell** es el espacio de trabajo nativo de Pi con subagentes, vista de cambios, revisión y ODD por defecto. Su paquete npm sigue llamándose `gentle-pi`.
- **ODD**: Authorize, Explore, Resolve uncertainty, Classify, Track (antes de la primera escritura), Implement (tarea a tarea) y Close.
- Tareas dimensionadas por comprensión, riesgo y capacidad de retomarse. Las pequeñas se hacen en línea; las grandes crean `odd/tasks/<feature>.md` (objetivo, problema, motivo, alcance, restricciones, checklist con IDs estables y criterios de aceptación, evidencia, progreso, siguiente paso), con copia en Engram `odd/<feature>/tasks`.
- Test-first por defecto si hay test ejecutable y resultado esperado claro.
- RDD activo por defecto (`gentle-ai review mode disable` para quitarlo); la profundidad la decide el riesgo del cambio congelado.
- Delegación solo con motivo: exploración, escritores en paralelo con superficies disjuntas, relevo por sesión larga o **verificador independiente para cambios de alto riesgo**. En este proyecto, **todo cambio en `internal/finance` es de alto riesgo** y lleva verificador independiente.
- Push, PR y merge son decisiones aparte del commit.

Puede cambiar entre versiones: compruébalo en H-01.

### 10.2 Instalación y comprobaciones (H-01)

```bash
brew install gentleman-programming/tap/gentle-ai
gentle-ai && gentle-ai doctor && gentle-ai review mode status
npm i -g gentle-pi
gentle-shell              # dentro: /gentle:status y /gentle:doctor
# tras actualizar gentle-ai: gentle-ai sync
```

### 10.3 Backlog y ODD

| Este documento | ODD |
|---|---|
| Una tarea (`FIN-01`) | Una funcionalidad, con su documento si es grande |
| Una ola | Un lote autorizado por ti en Authorize |
| Criterios de aceptación | Checklist con IDs estables |
| Superficie de edición | Permite escritores en paralelo sin solaparse |
| "Cuándo parar y preguntar" | Resolve uncertainty |

### 10.4 Plantilla del documento de funcionalidad

````markdown
# <ID>: <título>

## Objetivo

## Problema y motivo

## Alcance
- Dentro:
- Fuera:

## Restricciones
Principios de la sección 3 que aplican.

## Superficie de edición (nuestra)

## Riesgo (nuestra)
bajo | medio | alto. Alto si toca internal/finance.

## Checklist
- [ ] <ID>.1 ... · Criterio: ...

## Evidencia de verificación

## Progreso

## Siguiente paso

## Log
````

### 10.5 Review gate tras cada implementación (capa propia)

1. Tarea implementada y `make check` en verde.
2. Se congela `<base>..<sha>`.
3. Subagente revisor de contexto limpio y **solo lectura**, con otro modelo si es posible.
4. Orden: corrección; **P-04, P-05, P-06 y P-07** (transparencia, atribución, nunca inventar un índice, decimal exacto); contrato JSON y códigos de salida; fechas y meses de referencia; simplicidad; tests (¿hay caso de referencia independiente?); rendimiento solo con evidencia.
5. Informe en `odd/reviews/<id>.md` con severidades, archivo y línea, propuesta y veredicto.
6. Una sola ronda de correcciones. Refactores al backlog como `REF-<n>`.
7. En tareas de `internal/finance`, además, **verificador independiente** de ODD.

### 10.6 Entrega

Ramas `hc/<id>-descripcion`, un PR por tarea. Merge siempre humano (H-02).

---

## 11. AGENTS.md (copiar tal cual)

````markdown
# AGENTS.md · hipoteca-cli

Lee `docs/ARRANQUE.md`, `docs/series.md` y `docs/calculos.md` antes de tu primera tarea.

## Qué es
CLI en Go que lee los tipos de referencia oficiales del Banco de España y calcula revisiones, cuadros de amortización, amortizaciones anticipadas y escenarios de una hipoteca. Solo lectura frente a terceros.

## Cómo trabajar
- ODD (gentle-ai / gentle-shell): Authorize, Explore, Resolve uncertainty, Classify, Track, Implement, Close.
- Una tarea por vez por su ID. Si es grande, `odd/tasks/<id>-<slug>.md` antes de escribir código.
- Test-first.
- `internal/finance` es puro: sin red, sin E/S, sin reloj. Todo cambio ahí es de riesgo alto.
- Solo `internal/bde` hace HTTP.
- `internal/model` y `schemas/` son el contrato: cambios en PR aparte con `schema_version`.
- Antes de cerrar: `make check` en verde y review gate con informe en `odd/reviews/<id>.md`. Una ronda de correcciones.
- Rama `hc/<id>-descripcion`. Commit y PR. No hagas merge a `main`.

## Reglas duras
- Nada de float64 para dinero ni tipos. Decimal exacto.
- Nunca inventes un valor de índice. Si no está publicado, `index_not_published`. Los escenarios solo con `--assume` y marcados como `assumed`.
- Toda salida con cálculos incluye sus entradas y la atribución exacta del Banco de España con fecha de actualización.
- Nunca supongas datos de la escritura (mes de referencia, redondeo, suelo, comisión): si faltan, `loan_incomplete`.
- Ningún texto recomienda una decisión financiera.
- Los datos del préstamo nunca salen de la máquina salvo `--with-impact` activado por el usuario.
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
````

---

## 12. CI

Igual que en trafico-cli: matriz Linux, macOS y Windows con `make check`, un job que exige `odd/reviews/<id>.md` cuando el título del PR lleva ID, y un job nocturno que llama a la API real, valida las series de `series.yaml` y abre una issue automática si algo cambia.

---

## 13. Riesgos

| Riesgo | Mitigación |
|---|---|
| Un error en el cálculo da cifras falsas a alguien que toma decisiones con dinero | `internal/finance` puro, decimal exacto, casos de referencia independientes, tests de propiedades, verificador independiente en cada cambio |
| Diferencias de céntimos con el banco | Convenciones explícitas y configurables, `loan sync` con el capital real, aviso en README y skill |
| Suponer mal el mes de referencia o el redondeo de la escritura | Campos obligatorios; `loan_incomplete` si faltan |
| El Banco de España cambia códigos o formato | Códigos en config, código 7, tests contra respuestas reales, job nocturno |
| Interpretarse como asesoramiento | P-08, textos revisados en el review gate, aviso en README y `doctor` |
| Uso indebido de la marca del Banco de España | `not_endorsed`, README sin logos ni sugerencia de respaldo |
| Fuga de datos del préstamo en avisos | Sin datos del préstamo por defecto; `--with-impact` explícito con advertencia |

---

## 14. Backlog

Prefijos: H, SPIKE, OPS, CORE, BDE, FIN, LOAN, CMD, WATCH, AGENT, QA, REL, REF.

### Ola 0: preparación

| ID | Tarea | Depende | Criterio de aceptación |
|---|---|---|---|
| H-01 | Instalar y comprobar gentle-ai y gentle-shell | | Notas en `docs/adr/0001-workflow.md` |
| H-02 | Proteger `main` | OPS-01 | Regla activa |
| H-03 | Reunir 2 o 3 casos reales anonimizados (tu propia hipoteca u otros) con recibos de revisión para usarlos como casos de referencia | | `testdata/finance/real-*.json` sin datos identificativos |
| SPIKE-01 | Localizar en HERMES los códigos de serie de cada índice de 2.1; llamar a `favoritas` y `listaSeries`; documentar campos, frecuencias, cabeceras y cómo se marcan meses no publicados; guardar respuestas | | `docs/series.md`, `internal/bde/series.yaml`, `testdata/bde/` |
| SPIKE-02 | Redactar `docs/calculos.md`: fórmulas, convenciones, redondeos y 10 casos de referencia calculados en hoja de cálculo o `numpy-financial` | | Documento y `testdata/finance/` revisados por ti |

### Ola 1: núcleo

| ID | Tarea | Depende | Criterio de aceptación |
|---|---|---|---|
| OPS-01 | Repo con estructura 8.3, Makefile, CI, AGENTS.md | | `make check` verde en los tres SO |
| CORE-01 | Salida, sobre de error, códigos de salida, atribución, formato decimal en JSON | OPS-01 | Tests e2e de stdout limpio, códigos y atribución |
| CORE-02 | Config, XDG, escrituras atómicas | OPS-01 | Tests con HOME temporal |
| BDE-01 | Cliente de la API, caché, `series.yaml` | SPIKE-01, CORE-02 | Tests contra respuestas reales; códigos 5, 6 y 7 |
| FIN-01 | Cuota, cuadro de amortización y tipo 0 % (riesgo alto) | SPIKE-02 | Casos de referencia y propiedades en verde; verificador independiente |
| OPS-02 | Review gate en CI y plantillas; linter anti-float en `internal/finance` | OPS-01 | PR sin informe falla; un `float64` en finance falla |

### Ola 2: préstamo y revisión

| ID | Tarea | Depende | Criterio de aceptación |
|---|---|---|---|
| LOAN-01 | Modelo y validación del préstamo; `loan add/edit/show/list/remove/sync` | CORE-02 | Préstamo sin campo de escritura da `loan_incomplete` |
| FIN-02 | Revisión: mes de referencia, suelo, redondeo, mixta (riesgo alto) | FIN-01, BDE-01 | Casos de referencia y los reales de H-03 coinciden al céntimo o con la tolerancia documentada |
| CMD-01 | `index list/show/latest`, `revision`, `schedule`, `calc` | FIN-02, LOAN-01 | Golden; `index_not_published` y `--assume` probados |

### Ola 3: decisiones

| ID | Tarea | Depende | Criterio de aceptación |
|---|---|---|---|
| FIN-03 | Amortización anticipada en ambos modos con comisión (riesgo alto) | FIN-01 | Casos de referencia; propiedad "amortizar nunca aumenta intereses totales" |
| FIN-04 | Escenarios y comparativa de índices (riesgo alto) | FIN-02 | Casos de referencia con históricos reales guardados |
| CMD-02 | `amortize`, `scenario`, `compare` | FIN-03, FIN-04 | Golden; ningún texto de recomendación |

### Ola 4: avisos

| ID | Tarea | Depende | Criterio de aceptación |
|---|---|---|---|
| WATCH-01 | `watch add/list/remove/check/run`, evento `index.published`, sinks, `--with-impact` con advertencia | CMD-01 | Un mes nuevo en la respuesta simulada produce exactamente un evento; sin `--with-impact` el aviso no contiene datos del préstamo |
| WATCH-02 | `watch service install/uninstall/status` | WATCH-01 | Linux y macOS sin restos |

### Ola 5: agentes y publicación

| ID | Tarea | Depende | Criterio de aceptación |
|---|---|---|---|
| AGENT-01 | `skills`, `schema` y `doctor` completo | Olas 2 a 4 | Un agente sin contexto resuelve las cuatro recetas de 6.5 solo con `skills get` |
| AGENT-02 | `docs/cli-spec.md` generado; README con atribución y avisos | AGENT-01 | Referencia al día con el binario |
| REL-01 | GoReleaser, checksums, `install.sh`, tap, Scoop, `update` | AGENT-02 | `v0.1.0` instalable en una línea |
| QA-01 | Pase de arquitectura final | REL-01 | `odd/reviews/ola-5.md` y tareas REF |

**Sugerencia de corte:** `v0.1.0` tras las Olas 1 a 3. Los avisos en `v0.2.0`.

---

## 15. Decisiones abiertas

| ID | Decisión | Recomendación |
|---|---|---|
| D-01 | Comandos en inglés o en español | Inglés, como el resto de tus CLIs; `pretty` en español |
| D-02 | Nombre del binario | `hipoteca` |
| D-03 | `shopspring/decimal` o `math/big.Rat` | `shopspring/decimal` por legibilidad; decidir en FIN-01 |
| D-04 | ¿Cubrir préstamos que no son hipotecas (personales, coche)? | No en la v1: `calc` ya sirve para un cálculo suelto |
| D-05 | Modelos del implementador, el revisor y el verificador | Distintos; el verificador de `internal/finance`, el más capaz que tengas |

---

## 16. Prompt de arranque para gentle-shell

```
Lee docs/ARRANQUE.md, docs/series.md, docs/calculos.md y AGENTS.md.
Trabajamos con ODD: Authorize, Explore, Resolve uncertainty, Classify,
Track, Implement, Close.

Autorizo la Ola 1 (sección 14). Test-first activo.

Orden:
1. OPS-01.
2. Cuando cierre, en paralelo si las superficies son disjuntas:
   CORE-01, CORE-02 y FIN-01.
3. Después: BDE-01 y OPS-02.

Reglas:
- Si la tarea es grande, su documento odd/tasks/<id>-<slug>.md con la plantilla
  de la sección 10.4 antes de escribir código.
- FIN-01 es de riesgo alto: verificador independiente además del review gate.
- Nunca modifiques testdata/finance/ para que pase un test.
- make check en verde y review gate (sección 10.5) antes de cerrar.
- Ningún test de PR toca la red.
- Commit en ramas hc/<id>-descripcion y PR. No hagas merge a main.
- Para y pregunta ante cualquier punto de "Cuándo parar y preguntar".

Al terminar cada tarea: qué cambió, cómo lo verificaste y qué dijo la review,
en tres líneas.
```

---

## 17. Fuentes consultadas

- [Tipos de referencia oficiales del mercado hipotecario (Banco de España, Portal del Cliente Bancario)](https://clientebancario.bde.es/pcb/es/menu-horizontal/podemosayudarte/tiposinteres/guia_textual/tiposinteresreferenciaotrostiposfrecuentes/tipo_referencia_oficial_mercado_hipotecario.html)
- [API de estadísticas del Banco de España](https://www.bde.es/webbe/en/estadisticas/recursos/api-estadisticas-bde.html)
- [Términos de uso de las estadísticas del Banco de España](https://www.bde.es/wbe/es/estadisticas/recursos/terminos/terminos-de-uso.html)
- [tidyBdE (paquete de R)](https://github.com/ropenspain/tidyBdE)
- Patrón de CLI para agentes: [booksy-cli](https://github.com/GodGMN/booksy-cli), [vodafone-cli](https://github.com/luisgonzaleznf/vodafone-cli), [wallapop-cli](https://github.com/Microck/wallapop-cli)
- [gentle-ai](https://github.com/Gentleman-Programming/gentle-ai) · [gentle-shell](https://github.com/Gentleman-Programming/gentle-shell)
