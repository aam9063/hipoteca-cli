# Series del Banco de España (resultado de SPIKE-01)

Contrato verificado de la API de estadísticas del Banco de España y catálogo de códigos de
serie que usa `hipoteca-cli`. Todo lo que hay aquí se comprobó contra el servicio real el
**2026-10-28**; las capturas están en `testdata/bde/`.

Este documento manda sobre la sección 2.2 del arranque donde discrepen: el arranque se
escribió antes de tocar la API.

> Fuente: Sitio web del Banco de España (www.bde.es)
>
> Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)
>
> Este proyecto no está afiliado, patrocinado ni respaldado por el Banco de España.

---

## 1. Catálogo de índices

`id` es el identificador estable que aceptan los comandos (`--index`, `index show`, ...).
Cambiarlo es un cambio de contrato (P-09). `código` es el de la serie del Banco de España
y se puede sobrescribir en config si el Banco lo cambia (arranque §7).

### 1.1 Euríbor

Medias mensuales calculadas por EMMI. Tipo de referencia oficial del mercado hipotecario.

| `id` | Código | Nombre | Frecuencia | Decimales | Primer valor |
|---|---|---|---|---|---|
| `euribor-1w` | `D_1NBAS972` | Euríbor a 1 semana | Mensual | 3 | — |
| `euribor-1m` | `D_1NBAC972` | Euríbor a 1 mes | Mensual | 3 | — |
| `euribor-3m` | `D_1NBAD972` | Euríbor a 3 meses | Mensual | 3 | — |
| `euribor-6m` | `D_1NBAE972` | Euríbor a 6 meses | Mensual | 3 | — |
| `euribor-1y` | `D_1NBAF472` | Euríbor a 1 año | Mensual | 3 | Ene 1999 |

`euribor-1y` es el que usan la mayoría de las hipotecas variables en España.

### 1.2 Otros índices oficiales del mercado hipotecario

| `id` | Código | Nombre | Frecuencia | Decimales | Primer valor |
|---|---|---|---|---|---|
| `irph` | `D_1T9H0000` | IRPH conjunto de entidades de crédito: tipo medio de adquisición de vivienda libre, más de 3 años, España | Mensual | 3 | Nov 1990 |
| `irs-5y` | `D_1T9H0011` | IRS (permuta de intereses) a 5 años | Mensual | 3 | Mar 2009 |
| `deuda-publica-2a6` | `D_1T9H0004` | Rendimiento interno de la deuda pública entre 2 y 6 años, mercado secundario | Mensual | 3 | — |
| `hipotecario-ue-1a5` | `D_1T9H0010` | Tipo medio de los préstamos hipotecarios entre 1 y 5 años, conjunto de entidades de la UEM | Mensual | 3 | — |

### 1.3 €STR

Los cinco códigos de la familia `*572` comparten el mismo `descripcionCorta`
("Tipos de interés. UEM. Mercado monetario. ¤STR"), así que **el plazo no se puede deducir
de la respuesta de `favoritas`**. El plazo real solo aparece en
`informacion[].[Descripción]` de `listaSeries`, que es de donde se fijó esta tabla:

| `id` | Código | Plazo | `informacion[].[Descripción]` (extracto) |
|---|---|---|---|
| `estr-1w` | `D_1NBAS572` | 1 semana | `... TIPO DE INTERÉS A UNA SEMANA ... ESTER ...` |
| `estr-1m` | `D_1NBAC572` | 1 mes | `... A UN MES ...` |
| `estr-3m` | `D_1NBAD572` | 3 meses | `... A TRES MESES ...` |
| `estr-6m` | `D_1NBAE572` | 6 meses | `... A SEIS MESES ...` |
| `estr-1y` | `D_1NBAF572` | 1 año | `... A 1 AÑO ...` |

Primer valor: Oct 2020. Frecuencia mensual, 3 decimales.

El patrón de sufijos se repite en toda la familia de Euríbor (`*972`) y €STR (`*572`):
`S` = 1 semana, `C` = 1 mes, `D` = 3 meses, `E` = 6 meses, `F` = 1 año. **No se debe
usar ese patrón para adivinar un índice**: se usa solo como señal para saber dónde mirar.
El nombre que se muestra al usuario sale de `series.yaml`.

### 1.4 Series auxiliares del IRPH (no son índices)

Existen en la API y son útiles para `doctor` y para explicar el cálculo, pero **no son
tipos**: no se ofrecen como `--index` ni entran en ningún cálculo de cuota.

| Código | Qué es | `simbolo` | `decimales` | Valor 2026-09 |
|---|---|---|---|---|
| `D_1T9H0000A` | Importe agregado de los préstamos usados para calcular el IRPH | `K  EUR` | 0 | 4447519 |
| `D_1T9H0000B` | Número de operaciones | *(ausente)* | 0 | 24498 |
| `D_1T9H0000C` | Entidades contribuidoras | `Entidades` | 0 | 21 |

Dos cosas que esto enseña sobre el contrato:

- **`simbolo` puede faltar** en una respuesta válida (`D_1T9H0000B` no lo trae). El cliente
  no puede exigirlo.
- `decimales: 0` existe: no todas las series son porcentajes con 3 decimales.

**Las fechas de publicación en el BOE no están en la API.** Los códigos `D_1TFECHABOE`,
`D_1TFECHABOE2`, `D_1TFECHABOE3` y `D_1TFECHABOE4` aparecen en el CSV del Boletín
Estadístico, pero `favoritas` los devuelve como `errNum: 404` (ver
`testdata/bde/favoritas-series-auxiliares.json`). Si la CLI necesita la fecha del BOE,
tiene que sacarla del `fechaValor` de la serie de tipo, no de esas series.

---

## 2. Endpoints

Base: `https://app.bde.es/bierest/resources/srdatosapp/`

| Uso | Endpoint |
|---|---|
| Último valor de una o varias series | `favoritas?idioma=es&series=<códigos separados por comas>` |
| Histórico | `listaSeries?idioma=es&series=<códigos>&rango=<rango>` |

Sin token, sin cabecera de autenticación, sin cookies necesarias. `access-control-allow-origin: *`.

### 2.1 Parámetro `idioma` — obligatorio

`es` o `en`. Afecta a `descripcion` y `descripcionCorta` (no a los códigos ni a los
valores). Sin `idioma` la respuesta es un error 412 con
`"El parametro idioma es obligatorio"`. La CLI usa `idioma=es` para su caché y muestra
siempre el nombre de `series.yaml`, no el de la respuesta.

### 2.2 Parámetro `series`

Lista de códigos separados por comas. En los códigos, `#` va como `%23` y `%` como `%25`
(arranque §2.2). Si falta: 412 `"El parametro series es obligatorio"`.

`favoritas` **no** devuelve un listado de series disponibles: sin `series` no hay nada que
pedir. El catálogo de códigos está en este documento y en `internal/bde/series.yaml`.

### 2.3 Parámetro `rango` (solo `listaSeries`)

Probado exhaustivamente:

| `rango` | Resultado |
|---|---|
| `30M` | 31 puntos (30 meses + el mes en curso) |
| `60M` | 61 puntos |
| `MAX` | Toda la serie (333 puntos en `euribor-1y`, desde 1999-01) |
| `<año>` con datos (`2025`, `2026`) | Los meses publicados de ese año |
| *(vacío)* | Equivale a `30M` |
| `12M`, `24M`, `36M`, `120M` | **412** `"El rango 12M no es correcto"` |
| Un año futuro (`2099`) | 412 |

Consecuencia para la CLI: los rangos de `--range` son **30M, 60M, MAX y un año**, y `30M`
no son 30 puntos sino 31. `hipoteca index show --range 36M` no puede traducirse
literalmente a la API: se resuelve con `MAX` (o el año correspondiente) y se recorta en
cliente. Esa decisión es de CMD-01.

---

## 3. Forma de las respuestas

### 3.1 `favoritas` (éxito) — array

```json
[
  {
    "serie": "D_1NBAF472",
    "descripcionCorta": "Euribor a un año",
    "codFrecuencia": "M",
    "decimales": 3,
    "simbolo": "%",
    "tendencia": "+",
    "fechaValor": "2026-09-01T08:15:00Z",
    "valor": 3.247
  }
]
```

El orden del array sigue el orden de los códigos pedidos.

### 3.2 `listaSeries` (éxito) — array con un objeto por serie

```json
[
  {
    "serie": "D_1NBAF472",
    "descripcion": "Tipos de interés. UEM. Mercado monetario. Euríbor. A 12 meses",
    "descripcionCorta": "Euribor a un año",
    "codFrecuencia": "M",
    "decimales": 3,
    "simbolo": "%",
    "informacion": [{ "titulo": "Nombre", "descripcion": "..." }],
    "fechaInicio": "1999-01-01T09:15:00Z",
    "fechaFin": "2026-09-01T08:15:00Z",
    "fechas": ["2026-09-01T08:15:00Z", "2026-08-01T08:15:00Z"],
    "valores": [3.247, 2.954]
  }
]
```

- `informacion[]` es una lista de pares `{titulo, descripcion}` con metadatos de la serie
  (Nombre, Descripción larga, Unidades, Decimales, Número de observaciones, Primer valor,
  Último valor, Valor mínimo, Valor máximo).
- `fechas` y `valores` van **en el mismo orden** y **descendente** (lo más nuevo primero).
- `fechaInicio` y `fechaFin` son la primera y la última observación de la serie completa,
  no del rango pedido.

### 3.3 Tipos numéricos: el detalle que rompe P-07

**`valor` y `valores` son números JSON, no cadenas.** Decodificarlos con
`encoding/json` en un campo `float64` es una violación de P-07 en el borde mismo del
sistema: `3.45` no es representable en binario y ese error se propaga a cuotas.

Regla para `internal/bde`: decodificar esos campos como `json.Number` y convertir
inmediatamente a `shopspring/decimal` con `decimal.NewFromString` (que respeta la cadena
original del JSON, no el valor binario). `decimales` (en la respuesta) indica cuántos
decimales tiene el dato y sirve para validar y para formatear; no se usa para redondear.

`decimales`, `codFrecuencia` y `simbolo` son del propio JSON (`3`, `"M"`, `"%"`).
**`simbolo` y `tendencia` son opcionales**: `D_1T9H0000B` (número de operaciones del IRPH)
no trae `simbolo`. Un cliente que los exija se rompe con una respuesta válida.

---

## 4. Serie y meses no publicados (base de `index_not_published`)

Un mes que el Banco de España todavía no ha publicado **no aparece** en `fechas`. No hay
centinela, no hay `null`, no hay hueco: la serie simplemente termina antes.

Evidencia: `listaSeries-euribor-1y-2026.json`, descargado el 2026-10-28, devuelve 9 puntos
(2026-01 a 2026-09). Octubre de 2026 no está, y `fechaFin` es 2026-09-01.

Regla para FIN-02 y CMD-01:

1. El mes de referencia pedido se busca por año-mes en `fechas`.
2. Si no está, `index.status = "not_published"` y el comando falla con código 4
   `index_not_published` y el `hint` de `--assume`.
3. Nunca se usa el último valor disponible como si fuera el del mes pedido, ni se
   interpola. Eso es P-06.
4. `--assume <valor>` produce `index.status = "assumed"` y un aviso en `warnings`.

Corolario para `watch` (WATCH-01): "hay un mes nuevo" se detecta comparando el último
año-mes de `fechas` con el último ya visto en la base de datos local.

---

## 5. Fechas y horas

- `fechaValor`, `fechaInicio` y `fechaFin` son ISO 8601 en UTC.
- La **hora no es significativa y no es estable**: en la misma descarga, enero-marzo de
  2026 vienen con `T09:15:00Z` y abril-septiembre con `T08:15:00Z`. Solo el año-mes
  identifica la observación.
- El día es siempre `01` en las series mensuales.
- `hipoteca-cli` compara y ordena por año-mes, nunca por el instante completo.

---

## 6. Errores: mirar el cuerpo, no el status

Los errores de validación llegan con **HTTP 200** y un cuerpo distinto. Este es el punto
donde un cliente ingenuo se rompe.

| Caso | Status | Cuerpo | Forma |
|---|---|---|---|
| Serie inexistente en `listaSeries` | 200 | `{"errNum":412,"errMsgUsr":"Error de validación en la solicitud","errMsgDebug":"La serie XX no existe"}` | objeto |
| Rango inválido | 200 | `{"errNum":412,...,"errMsgDebug":"El rango 12M no es correcto"}` | objeto |
| Falta `idioma` | 200 | `{"errNum":412,...,"errMsgDebug":"El parametro idioma es obligatorio"}` | objeto |
| Falta `series` | 200 | `{"errNum":412,...,"errMsgDebug":"El parametro series es obligatorio"}` | objeto |
| Serie inexistente en `favoritas` | 200 | `[{"codigo":"XX","errNum":404}]` | **array**, por elemento |
| Ruta inexistente | 400 | HTML del sitio, no JSON | HTML |

Reglas de traducción para `internal/bde` → `errs` (arranque §6.4):

- `favoritas` es un array: se valida **por elemento**. Los elementos con `errNum` se
  separan; si el código lo pidió la CLI (y por lo tanto debería existir) es un fallo de
  catálogo → código 7 `api_changed` con enlace a issue, no `not_found`. Si el código lo
  escribió el usuario, es código 4 `not_found`.
- `listaSeries` con `errNum` 412 y `errMsgDebug` "no existe" → código 4 `not_found`.
- Cualquier otro `errNum` o una forma inesperada → código 7 `api_changed`.
- Un cuerpo que no es JSON → código 7 `api_changed`.
- Timeout, 5xx, 429 → código 5 `upstream_unavailable` / `rate_limited`.

## 7. Transporte

- Todas las respuestas vienen **gzip**: `content-encoding: gzip` y
  `vary: Accept-Encoding`. El cliente Go debe dejar que `net/http` lo descomprima
  (Transport por defecto con `Accept-Encoding: gzip`).
- `content-type: application/json;charset=UTF-8`.
- No se observaron cabeceras de rate limit (`Retry-After`, `X-RateLimit-*`) en la
  descarga. Aun así el cliente debe tratar 429, 5xx y los cortes de conexión como código 5
  `upstream_unavailable`/`rate_limited`, con reintentos y backoff acotado: no hay
  documentación de límites (arranque §2.2) y el Banco puede aplicarlos sin avisar.
- Sin `User-Agent` propio no hubo bloqueo, pero se recomienda identificarse
  (`hipoteca-cli/<version>`), que es lo que permite al Banco distinguir un cliente
  correcto de un scraper.
- Los datos publicados de meses pasados **no cambian**: se cachean sin caducidad. El mes
  en curso y el último valor, 24 h (arranque §7).

## 8. Licencia y atribución

Los términos de uso de las estadísticas del Banco de España permiten el acceso y la
reutilización libres y gratuitos, también comercialmente, con estas condiciones:

- Citar la fuente. Datos sin procesar: `Fuente: Sitio web del Banco de España (www.bde.es)`.
  Datos procesados (todo lo que calcula esta CLI):
  `Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)`.
- Indicar la fecha de la última actualización del original cuando la tenga.
- No alterar el contenido ni los metadatos, ni cambiar su sentido.
- No insinuar que el Banco de España participa, patrocina o respalda la herramienta.
- El Banco puede cambiar, suspender o retirar series y formatos sin aviso.

Traducción a la CLI: bloque `attribution` en toda salida con datos (P-05),
`not_endorsed: true`, códigos de serie sobrescribibles desde config (arranque §7) y
código 7 cuando una serie desaparece.

## 9. Cómo se mantiene al día

- `internal/bde/series.yaml` es la fuente compilada; si un código cambia, se corrige ahí y
  en este documento en el mismo PR.
- El job nocturno de CI (arranque §12) llama a `favoritas` con todos los códigos del
  catálogo: si alguno desaparece o cambia de forma, abre una issue.
- Los fixtures de `testdata/bde/` se renuevan cuando cambia el contrato, actualizando la
  fecha de descarga de su README.
- Los meses nuevos **no** obligan a tocar nada: la serie crece sola.
