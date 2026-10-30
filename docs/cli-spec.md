# Referencia de la CLI

> **Este documento se genera desde el binario; no se edita a mano.** Describe el árbol
> de comandos, las banderas y el contrato de salida tal y como los conoce `hipoteca`.
> Para regenerarlo tras cambiar un comando:
>
>     go run ./scripts/generate-cli-spec
>
> Un test del paquete `internal/cli` falla si este fichero difiere de lo que genera el
> binario (AGENT-02, S2): una referencia que se desfasa del binario miente con autoridad.

Salvo que se indique lo contrario, todos los comandos están disponibles en `v0.1.0`.
`watch` queda para `v0.2.0` y se marca como previsto: no está disponible y no debe
usarse todavía.

## Comandos

## `hipoteca`

**Estado:** v0.1.0 (disponible).

Índices oficiales del Banco de España y cálculos de tu hipoteca

hipoteca lee los tipos de referencia oficiales del mercado hipotecario que publica
el Banco de España (Euríbor, IRPH y otros) y calcula revisiones de cuota, cuadros de
amortización, amortizaciones anticipadas y escenarios de una hipoteca.

La salida es JSON para que la use un agente. No es asesoramiento financiero: calcula y
compara, no recomienda. No está afiliada ni respaldada por el Banco de España.

Uso: `hipoteca`

### `hipoteca amortize`

**Estado:** v0.1.0 (disponible).

Efecto de una amortización anticipada: cuota menor, menos plazo o ambas

Calcula el efecto de amortizar anticipadamente, sobre un préstamo guardado (con
su id, o el único guardado) o sobre un cálculo suelto con --principal --rate
--months. En modo compare devuelve las dos opciones con sus cifras, sin
recomendar ninguna.

Con préstamo guardado, el capital pendiente, el tipo, los meses restantes y la
comisión salen del préstamo (con sync, manda el recibo): las banderas
--principal, --rate, --months y --fee-pct no se admiten en ese camino. Cuando el
importe agota el saldo, el préstamo cierra por el propio evento y la salida lo
marca con closes_at_event (el mes del cierre). En ese caso payment_after
significa cosas distintas según el modo: en reduce-payment es 0.00, porque no
queda plazo sobre el que recalcular una cuota; en reduce-term es la cuota
mantenida, que es la del propio mes del evento y sí se cobró. En ambos el
cálculo termina en ese mes: después del evento no existe ninguna cuota
posterior.

Uso: `hipoteca amortize [id] [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--amount` | `string` | `*(vacío)*` | importe a amortizar anticipadamente en euros, p. ej. 10000.00 |
| `--date` | `int` | `0` | mes del evento: número de la cuota (1, 2, 3…) tras la que se amortiza, contado desde el inicio del cálculo; no es una fecha de calendario, p. ej. 60 |
| `--fee-pct` | `string` | `0` | comisión por la amortización anticipada en tanto por ciento del importe, p. ej. 0.50 (por defecto 0; se paga aparte y no reduce el capital; con préstamo guardado se usa la comisión de la escritura y la bandera no se admite) |
| `--mode` | `string` | `*(vacío)*` | modo de la amortización: reduce-payment, reduce-term o compare (devuelve las dos opciones, sin recomendar) |
| `--months` | `int` | `0` | plazo total en meses, p. ej. 300 (no se admite con un préstamo guardado) |
| `--principal` | `string` | `*(vacío)*` | capital del préstamo en euros, p. ej. 150000.00 (no se admite con un préstamo guardado) |
| `--rate` | `string` | `*(vacío)*` | tipo nominal anual en tanto por ciento, p. ej. 3.10 (no se admite con un préstamo guardado) |

### `hipoteca calc`

**Estado:** v0.1.0 (disponible).

Cuota y totales de un cálculo suelto, sin guardar el préstamo

Uso: `hipoteca calc [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--months` | `int` | `0` | plazo en meses, p. ej. 300 |
| `--principal` | `string` | `*(vacío)*` | capital del préstamo en euros, p. ej. 150000.00 |
| `--rate` | `string` | `*(vacío)*` | tipo nominal anual en tanto por ciento, p. ej. 3.10 |

### `hipoteca compare`

**Estado:** v0.1.0 (disponible).

Compara tu préstamo con otro índice y diferencial usando los históricos reales

Recalcula la historia de tu préstamo con otro índice y otro diferencial, mes a mes
con los valores oficiales de cada revisión, y compara los dos bloques: la cuota
de cada revisión, el total pagado, el total de intereses y la diferencia por año.

La comparación es sobre el pasado: cubre hasta la última revisión cuyo mes de
referencia está publicado en los dos índices, y lo declara en coverage. Si el
plazo sigue más allá, la salida lo avisa: el resto del plazo no se proyecta.

La diferencia es siempre alternativa − propia: una diferencia positiva significa
que con el índice alternativo se habría pagado más que con el propio. Con un
préstamo fijo el bloque propio es una recomposición hipotética y va marcado
como assumed: un fijo no tiene revisiones de tipo.

Con un préstamo guardado (su id, o el único guardado), el capital, el plazo, el
tramo inicial, el suelo y el redondeo salen del préstamo: --index y --spread son
el índice alternativo y su diferencial. Sin préstamo guardado, el cálculo es un
préstamo hipotético en tramos anuales con --principal y --months, y los valores
del índice son supuestos tuyos con --index-values (marcados como assumed).

Cada revisión usa el índice de su propio mes de referencia; si un mes del índice
alternativo no está publicado el comando responde index_not_published: no
interpola ni inventa valores. La salida compara, no recomienda.

Uso: `hipoteca compare [id] [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--floor` | `string` | `*(vacío)*` | suelo del tipo en tanto por ciento, p. ej. 2.00 (opcional; sólo sin préstamo guardado) |
| `--from` | `string` | `*(vacío)*` | mes desde el que se muestran y comparan las revisiones, YYYY-MM, p. ej. 2025-01 (el recálculo y los totales empiezan siempre en el inicio del préstamo) |
| `--index` | `string` | `*(vacío)*` | id del índice alternativo con el que comparar, p. ej. irph |
| `--index-values` | `string` | `*(vacío)*` | valores del índice supuestos, separados por comas: uno por revisión con préstamo guardado, o uno por periodo de 12 meses sin él, p. ej. 2.0,2.2 (cada uno se marca assumed) |
| `--months` | `int` | `0` | plazo total en meses, p. ej. 240 (sólo sin préstamo guardado) |
| `--own-spread` | `string` | `0` | diferencial del bloque propio en tanto por ciento, p. ej. 0.50 (sólo sin préstamo guardado; con él sale del préstamo) |
| `--principal` | `string` | `*(vacío)*` | capital en euros, p. ej. 150000.00 (sólo sin préstamo guardado) |
| `--rounding` | `string` | `none` | redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05 (sólo sin préstamo guardado; con él sale del préstamo) |
| `--spread` | `string` | `*(vacío)*` | diferencial del índice alternativo en tanto por ciento, p. ej. 1.00 |
| `--to` | `string` | `*(vacío)*` | mes hasta el que se comparan las revisiones, YYYY-MM, p. ej. 2026-01: acota también el cálculo y los totales, que corren hasta la revisión siguiente a la última comparada; no puede ser posterior al último mes publicado: la comparación sólo cubre meses publicados |

### `hipoteca config`

**Estado:** v0.1.0 (disponible).

La configuración local: verla y cambiar overrides y sinks

Uso: `hipoteca config`

#### `hipoteca config get`

**Estado:** v0.1.0 (disponible).

El valor de una clave: indices.<id>, sinks.<nombre>.type o sinks.<nombre>.url

Uso: `hipoteca config get <clave>`

#### `hipoteca config set`

**Estado:** v0.1.0 (disponible).

Declara o cambia una clave: un sink nuevo o un código de serie

Declara o cambia una clave de la configuración. Las claves admitidas:

  sinks.<nombre>.url     la URL del webhook de avisos (declara el sink si no existe)
  sinks.<nombre>.type    el tipo del sink; en v1, "webhook"
  indices.<id>           el código de serie del Banco para un índice del catálogo

El valor se valida ANTES de escribir: una URL que no sea http(s) con host, un
tipo que no sea webhook o un índice fuera del catálogo son errores de uso y la
config queda intacta. Los préstamos se editan con "hipoteca loan add/edit",
no desde aquí.

Uso: `hipoteca config set <clave> <valor>`

#### `hipoteca config show`

**Estado:** v0.1.0 (disponible).

La configuración completa (las URLs de los sinks salen redactadas)

Uso: `hipoteca config show`

### `hipoteca doctor`

**Estado:** v0.1.0 (disponible).

El estado de la instalación: rutas, permisos, catálogo, préstamos y red

Responde el estado completo de la instalación: versión y commit del binario,
las rutas de config, datos y caché con su permiso, los índices del catálogo
con su código de serie, los préstamos guardados —y, si alguno está
incompleto, el campo de la escritura que le falta— y una única comprobación
de conectividad con el Banco de España.

Con --offline no se toca la red: la salida lo declara con network.checked
false. Doctor es un diagnóstico: informa de lo que encuentra y sale 0.

Uso: `hipoteca doctor [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--offline` | `bool` | `false` | no comprobar la red: la salida declara network.checked false y nada toca al Banco de España |

### `hipoteca index`

**Estado:** v0.1.0 (disponible).

Índices de referencia del Banco de España: lista, último valor e histórico

Uso: `hipoteca index`

#### `hipoteca index latest`

**Estado:** v0.1.0 (disponible).

Último valor publicado de uno o varios índices

Uso: `hipoteca index latest <id>...`

#### `hipoteca index list`

**Estado:** v0.1.0 (disponible).

Todos los índices del catálogo con su último valor publicado

Uso: `hipoteca index list`

#### `hipoteca index show`

**Estado:** v0.1.0 (disponible).

Histórico mensual de un índice

Uso: `hipoteca index show <id> [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--range` | `string` | `30M` | rango del histórico: 30M, 60M, MAX o un año con datos (entre 1990 y el año en curso) |

### `hipoteca loan`

**Estado:** v0.1.0 (disponible).

Préstamos guardados en tu config local: alta, edición, borrado y sincronización

Uso: `hipoteca loan`

#### `hipoteca loan add`

**Estado:** v0.1.0 (disponible).

Guarda un préstamo nuevo en la config local

Uso: `hipoteca loan add <id> [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--fee-pct` | `string` | `*(vacío)*` | comisión de amortización anticipada en tanto por ciento del importe, p. ej. 0.50; usa 0 si tu escritura no la tiene |
| `--floor` | `string` | `*(vacío)*` | suelo del tipo en tanto por ciento, p. ej. 2.00; usa none o cadena vacía para sin suelo (dato de la escritura: no se supone) |
| `--index` | `string` | `*(vacío)*` | id del índice de revisión; "hipoteca index list" muestra los disponibles |
| `--initial-period-months` | `int` | `0` | meses a tipo fijo antes de la primera revisión, p. ej. 12 |
| `--initial-rate` | `string` | `*(vacío)*` | TIN anual % del periodo inicial, p. ej. 1.25 |
| `--interest-convention` | `string` | `monthly-nominal` | convención de interés (sólo se soporta monthly-nominal) |
| `--months` | `int` | `0` | plazo total en meses, p. ej. 360 |
| `--name` | `string` | `*(vacío)*` | nombre con el que reconoces el préstamo, p. ej. Casa |
| `--payment-day` | `int` | `0` | día del mes en que se paga la cuota, p. ej. 1 |
| `--payment-rounding` | `string` | `half-up-cents` | redondeo de la cuota (sólo se soporta half-up-cents) |
| `--principal` | `string` | `*(vacío)*` | capital original en euros, p. ej. 180000.00 |
| `--rate-rounding` | `string` | `*(vacío)*` | redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05 |
| `--reference-offset-months` | `int` | `0` | meses de índice antes del mes de revisión, según escritura, p. ej. 2 (0 es un valor válido) |
| `--revision-every-months` | `int` | `0` | meses entre revisiones, p. ej. 12 |
| `--spread` | `string` | `*(vacío)*` | diferencial sobre el índice en puntos, p. ej. 0.89 |
| `--spread-unbonified` | `string` | `*(vacío)*` | diferencial sin bonificaciones en puntos, p. ej. 1.39 (opcional) |
| `--start-date` | `string` | `*(vacío)*` | fecha de la primera cuota o de firma, según escritura (YYYY-MM-DD) |
| `--type` | `string` | `*(vacío)*` | tipo de préstamo: variable, mixta o fija |

#### `hipoteca loan edit`

**Estado:** v0.1.0 (disponible).

Cambia los campos indicados de un préstamo guardado

Uso: `hipoteca loan edit <id> [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--fee-pct` | `string` | `*(vacío)*` | comisión de amortización anticipada en tanto por ciento del importe, p. ej. 0.50; usa 0 si tu escritura no la tiene |
| `--floor` | `string` | `*(vacío)*` | suelo del tipo en tanto por ciento, p. ej. 2.00; usa none o cadena vacía para sin suelo (dato de la escritura: no se supone) |
| `--index` | `string` | `*(vacío)*` | id del índice de revisión; "hipoteca index list" muestra los disponibles |
| `--initial-period-months` | `int` | `0` | meses a tipo fijo antes de la primera revisión, p. ej. 12 |
| `--initial-rate` | `string` | `*(vacío)*` | TIN anual % del periodo inicial, p. ej. 1.25 |
| `--interest-convention` | `string` | `monthly-nominal` | convención de interés (sólo se soporta monthly-nominal) |
| `--months` | `int` | `0` | plazo total en meses, p. ej. 360 |
| `--name` | `string` | `*(vacío)*` | nombre con el que reconoces el préstamo, p. ej. Casa |
| `--payment-day` | `int` | `0` | día del mes en que se paga la cuota, p. ej. 1 |
| `--payment-rounding` | `string` | `half-up-cents` | redondeo de la cuota (sólo se soporta half-up-cents) |
| `--principal` | `string` | `*(vacío)*` | capital original en euros, p. ej. 180000.00 |
| `--rate-rounding` | `string` | `*(vacío)*` | redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05 |
| `--reference-offset-months` | `int` | `0` | meses de índice antes del mes de revisión, según escritura, p. ej. 2 (0 es un valor válido) |
| `--revision-every-months` | `int` | `0` | meses entre revisiones, p. ej. 12 |
| `--spread` | `string` | `*(vacío)*` | diferencial sobre el índice en puntos, p. ej. 0.89 |
| `--spread-unbonified` | `string` | `*(vacío)*` | diferencial sin bonificaciones en puntos, p. ej. 1.39 (opcional) |
| `--start-date` | `string` | `*(vacío)*` | fecha de la primera cuota o de firma, según escritura (YYYY-MM-DD) |
| `--type` | `string` | `*(vacío)*` | tipo de préstamo: variable, mixta o fija |

#### `hipoteca loan list`

**Estado:** v0.1.0 (disponible).

Lista los préstamos guardados con su marca de completo

Uso: `hipoteca loan list`

#### `hipoteca loan remove`

**Estado:** v0.1.0 (disponible).

Borra un préstamo guardado (irreversible; pide --yes)

Uso: `hipoteca loan remove [id] [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--yes` | `bool` | `false` | confirma el borrado: sin ella no se borra nada |

#### `hipoteca loan show`

**Estado:** v0.1.0 (disponible).

Muestra un préstamo guardado, con los campos de la escritura que falten

Uso: `hipoteca loan show [id]`

#### `hipoteca loan sync`

**Estado:** v0.1.0 (disponible).

Sincroniza el préstamo con tu último recibo: capital pendiente y tipo real

Uso: `hipoteca loan sync [id] [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--date` | `string` | `*(vacío)*` | fecha del recibo (YYYY-MM-DD), p. ej. 2026-10-01 |
| `--outstanding` | `string` | `*(vacío)*` | capital pendiente según el recibo en euros, p. ej. 152341.18 |
| `--rate` | `string` | `*(vacío)*` | TIN vigente según el recibo en tanto por ciento, p. ej. 3.12 |

### `hipoteca revision`

**Estado:** v0.1.0 (disponible).

Revisión de la cuota de un préstamo guardado: índice, TIN nuevo y cuota nueva

Calcula la próxima revisión de la cuota (o la de una fecha con --date): mes de
referencia según la escritura, valor del índice publicado por el Banco de España,
TIN nuevo con suelo y redondeo de escritura, y cuota nueva con el capital pendiente
y los meses restantes.

El valor del índice sale de la serie oficial; si el mes de referencia aún no está
publicado el comando responde index_not_published. Con --assume se calcula un
escenario con un valor supuesto, marcado como assumed y avisado en la salida:
no es un dato oficial.

Uso: `hipoteca revision [id] [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--assume` | `string` | `*(vacío)*` | valor del índice supuesto en tanto por ciento, p. ej. 2.35: calcula un escenario marcado como assumed en lugar de leer el valor publicado |
| `--date` | `string` | `*(vacío)*` | mes o fecha de la revisión (YYYY-MM o YYYY-MM-DD); por defecto, la próxima revisión posterior al mes en curso, p. ej. 2027-01 |

### `hipoteca scenario`

**Estado:** v0.1.0 (disponible).

Cuota y coste total con valores del índice supuestos por ti

Uso: `hipoteca scenario [id] [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--floor` | `string` | `*(vacío)*` | suelo del tipo en tanto por ciento, p. ej. 2.00 (opcional; sin él un índice negativo da un TIN negativo; con préstamo guardado se usa el de la escritura y la bandera no se admite) |
| `--index-values` | `string` | `*(vacío)*` | valores del índice supuestos, separados por comas, p. ej. 1.50,2,2.5,3 (cada uno se devuelve tal cual y se marca assumed) |
| `--months` | `int` | `0` | meses restantes del préstamo sobre los que se calcula la cuota, p. ej. 240 (no se admite con un préstamo guardado) |
| `--principal` | `string` | `*(vacío)*` | capital pendiente en euros, p. ej. 150000.00 (no se admite con un préstamo guardado) |
| `--rounding` | `string` | `none` | redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05 (con préstamo guardado se usa el de la escritura y la bandera no se admite) |
| `--spread` | `string` | `0` | diferencial sobre el índice en tanto por ciento, p. ej. 0.80 (con préstamo guardado se usa el de la escritura y la bandera no se admite) |

### `hipoteca schedule`

**Estado:** v0.1.0 (disponible).

Cuadro de amortización de un préstamo guardado

Construye el cuadro de amortización del préstamo guardado. Con un recibo
sincronizado (loan sync) parte del capital y del tipo reales del recibo; sin él,
del capital original y del tipo inicial — en un préstamo variable eso sólo
describe el tramo inicial, y la salida lo avisa.

--from y --to recortan las filas mostradas por mes (ambos incluidos); los totales
son siempre los del cuadro completo.

Uso: `hipoteca schedule [id] [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--from` | `string` | `*(vacío)*` | primer mes del tramo a mostrar (YYYY-MM o YYYY-MM-DD), p. ej. 2027-01 |
| `--to` | `string` | `*(vacío)*` | último mes del tramo a mostrar (YYYY-MM o YYYY-MM-DD), p. ej. 2027-12 |

### `hipoteca schema`

**Estado:** v0.1.0 (disponible).

El contrato de salida de los comandos: campos, tipos y códigos de salida

Describe el contrato de salida de los comandos: sus campos y tipos, y los
códigos de salida del contrato (§6.4). Se genera de los mismos tipos que los
comandos escriben en stdout, así que no puede desfasar del binario.

Sin argumento describe todos los comandos; con un nombre ("hipoteca schema
revision") describe sólo ese.

Uso: `hipoteca schema [comando]`

### `hipoteca skills`

**Estado:** v0.1.0 (disponible).

Recetas embebidas para agentes: qué comando ejecutar y cómo

Uso: `hipoteca skills`

#### `hipoteca skills get`

**Estado:** v0.1.0 (disponible).

La receta completa, en texto plano listo para ejecutar

Uso: `hipoteca skills get <id>`

#### `hipoteca skills list`

**Estado:** v0.1.0 (disponible).

Las recetas disponibles, con su id y una línea de para qué sirven

Uso: `hipoteca skills list`

### `hipoteca version`

**Estado:** v0.1.0 (disponible).

Muestra la versión, el commit y la fecha de compilación

Uso: `hipoteca version`

### `hipoteca watch`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Aviso de nueva publicación de un índice que sigues

Uso: `hipoteca watch`

#### `hipoteca watch add`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Vigila un índice y avisa cuando se publique un mes nuevo

Uso: `hipoteca watch add index <id> [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--sink` | `string` | `webhook` | nombre del sink de la config que recibe los avisos |
| `--with-impact` | `bool` | `false` | incluye en el aviso el cambio estimado de cuota de tu préstamo (P-03: ese dato saldrá de tu máquina hacia el sink) |

#### `hipoteca watch check`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Una pasada: avisa de los meses nuevos publicados desde la última

Uso: `hipoteca watch check`

#### `hipoteca watch list`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Vigilancias guardadas

Uso: `hipoteca watch list`

#### `hipoteca watch remove`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Borra una vigilancia y su registro de meses vistos

Uso: `hipoteca watch remove <id>`

#### `hipoteca watch run`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Bucle de vigilancia: una pasada de check cada intervalo, hasta Ctrl+C

Uso: `hipoteca watch run [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--interval` | `string` | `12h` | tiempo entre pasadas; mínimo 1h, por defecto 12h (los índices son mensuales) |

#### `hipoteca watch service`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Instala, retira o consulta el aviso como servicio del usuario

Uso: `hipoteca watch service`

##### `hipoteca watch service install`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Instala el aviso como servicio del usuario y lo activa

Uso: `hipoteca watch service install [flags]`

| Bandera | Tipo | Valor por defecto | Descripción |
|---|---|---|---|
| `--interval` | `string` | `12h` | cada cuánto hace una pasada el servicio (mínimo 1h) |

##### `hipoteca watch service status`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Dice si el servicio está instalado

Uso: `hipoteca watch service status`

##### `hipoteca watch service uninstall`

**Estado:** v0.2.0 (previsto: todavía no disponible).

Retira el servicio y no deja restos

Uso: `hipoteca watch service uninstall`

## Contrato de salida

Todos los comandos escriben **un único valor JSON** en stdout, con un salto final
(decisión 7): sin envoltorio, sin prosa mezclada. `skills get` y la ayuda explícita
(`--help`) son las únicas salidas en texto plano. Los importes y los tipos van como
cadenas decimales exactas (`"152341.18"`), nunca como float. `schema_version` es la
versión del contrato: 1.

En caso de error no hay resultado: el sobre de error va a **stderr** con un código de
salida estable (§6.4). La tabla de abajo es la misma que produce `hipoteca schema`;
el 0 de éxito no aparece en ella porque no es un error.

### Códigos de salida

| Código | Tipos del contrato |
|---|---|
| 1 | `internal` |
| 2 | `usage`, `loan_incomplete`, `cancelled`, `confirmation_required` |
| 4 | `not_found`, `index_not_published` |
| 5 | `upstream_unavailable`, `rate_limited` |
| 6 | `network` |
| 7 | `api_changed` |

### `index list`

Todos los índices del catálogo con su último valor publicado

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `indices` | `array` |  |
| `indices[].id` | `string` |  |
| `indices[].name` | `string` |  |
| `indices[].code` | `string` |  |
| `indices[].month` | `string` |  |
| `indices[].value` | `decimal` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `index show`

Histórico mensual de un índice

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `index` | `object` |  |
| `index.id` | `string` |  |
| `index.code` | `string` |  |
| `index.name` | `string` |  |
| `index.description` | `string` | sí |
| `range` | `string` |  |
| `observations` | `array` |  |
| `observations[].month` | `string` |  |
| `observations[].value` | `decimal` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `index latest`

Último valor publicado de uno o varios índices

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `indices` | `array` |  |
| `indices[].id` | `string` |  |
| `indices[].name` | `string` |  |
| `indices[].code` | `string` |  |
| `indices[].month` | `string` |  |
| `indices[].value` | `decimal` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `calc`

Cuota, intereses y total de un cálculo suelto

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `inputs` | `object` |  |
| `inputs.principal` | `decimal` |  |
| `inputs.annual_rate_pct` | `decimal` |  |
| `inputs.term_months` | `integer` |  |
| `payment` | `decimal` |  |
| `total_interest` | `decimal` |  |
| `total_paid` | `decimal` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `revision`

La revisión de un préstamo guardado

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `string` |  |
| `revision_date` | `string` |  |
| `index` | `object` |  |
| `index.id` | `string` |  |
| `index.reference_month` | `string` |  |
| `index.value` | `decimal` |  |
| `index.status` | `string` |  |
| `index.series` | `string` |  |
| `index.published_at` | `string` | sí |
| `rate` | `object` |  |
| `rate.previous` | `decimal` |  |
| `rate.spread` | `decimal` |  |
| `rate.raw` | `decimal` |  |
| `rate.after_floor` | `decimal` |  |
| `rate.after_rounding` | `decimal` |  |
| `rate.new` | `decimal` |  |
| `inputs` | `object` |  |
| `inputs.outstanding_principal` | `decimal` |  |
| `inputs.remaining_months` | `integer` |  |
| `inputs.outstanding_source` | `string` |  |
| `inputs.interest_convention` | `string` |  |
| `payment` | `object` |  |
| `payment.previous` | `decimal` |  |
| `payment.new` | `decimal` |  |
| `payment.difference_month` | `decimal` |  |
| `payment.difference_year` | `decimal` |  |
| `warnings` | `array` |  |
| `warnings[]` | `string` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `schedule`

El cuadro de amortización de un préstamo guardado

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `string` |  |
| `annual_rate_pct` | `decimal` |  |
| `start_date` | `string` |  |
| `months` | `integer` |  |
| `rows` | `array` |  |
| `rows[].month` | `integer` |  |
| `rows[].date` | `string` |  |
| `rows[].payment` | `decimal` |  |
| `rows[].interest` | `decimal` |  |
| `rows[].principal_repaid` | `decimal` |  |
| `rows[].outstanding_after` | `decimal` |  |
| `total_interest` | `decimal` |  |
| `total_paid` | `decimal` |  |
| `warnings` | `array` |  |
| `warnings[]` | `string` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `amortize`

El efecto de una amortización anticipada

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `inputs` | `object` |  |
| `inputs.loan` | `string` | sí |
| `inputs.principal` | `decimal` |  |
| `inputs.annual_rate_pct` | `decimal` |  |
| `inputs.term_months` | `integer` |  |
| `inputs.amount` | `decimal` |  |
| `inputs.after_month` | `integer` |  |
| `inputs.mode` | `string` |  |
| `inputs.fee_pct` | `decimal` |  |
| `reduce_payment` | `object` | sí |
| `reduce_term` | `object` | sí |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `scenario`

Cuota y coste total con valores del índice supuestos

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `inputs` | `object` |  |
| `inputs.loan` | `string` | sí |
| `inputs.principal` | `decimal` |  |
| `inputs.term_months` | `integer` |  |
| `inputs.spread` | `decimal` |  |
| `inputs.floor` | `decimal` | sí |
| `inputs.rate_rounding` | `string` |  |
| `values` | `array` |  |
| `values[].index_value` | `decimal` |  |
| `values[].assumed` | `boolean` |  |
| `values[].annual_rate_pct` | `decimal` |  |
| `values[].payment` | `decimal` |  |
| `values[].total_interest` | `decimal` |  |
| `values[].total_paid` | `decimal` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `compare`

El préstamo recalculado con otro índice

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `string` | sí |
| `from` | `string` | sí |
| `to` | `string` | sí |
| `coverage` | `object` |  |
| `coverage.from` | `string` |  |
| `coverage.to` | `string` |  |
| `own_index` | `object` |  |
| `own_index.index` | `object` |  |
| `own_index.index.id` | `string` |  |
| `own_index.index.code` | `string` |  |
| `own_index.index.status` | `string` |  |
| `own_index.spread` | `decimal` |  |
| `own_index.initial_rate_pct` | `decimal` |  |
| `own_index.initial_period_months` | `integer` |  |
| `own_index.revisions` | `array` |  |
| `own_index.revisions[].period` | `integer` |  |
| `own_index.revisions[].revision_date` | `string` | sí |
| `own_index.revisions[].reference_month` | `string` | sí |
| `own_index.revisions[].index_value` | `decimal` |  |
| `own_index.revisions[].assumed` | `boolean` |  |
| `own_index.revisions[].annual_rate_pct` | `decimal` |  |
| `own_index.revisions[].payment` | `decimal` |  |
| `own_index.total_paid` | `decimal` |  |
| `own_index.total_interest` | `decimal` |  |
| `alternative_index` | `object` |  |
| `alternative_index.index` | `object` |  |
| `alternative_index.index.id` | `string` |  |
| `alternative_index.index.code` | `string` |  |
| `alternative_index.index.status` | `string` |  |
| `alternative_index.spread` | `decimal` |  |
| `alternative_index.initial_rate_pct` | `decimal` |  |
| `alternative_index.initial_period_months` | `integer` |  |
| `alternative_index.revisions` | `array` |  |
| `alternative_index.revisions[].period` | `integer` |  |
| `alternative_index.revisions[].revision_date` | `string` | sí |
| `alternative_index.revisions[].reference_month` | `string` | sí |
| `alternative_index.revisions[].index_value` | `decimal` |  |
| `alternative_index.revisions[].assumed` | `boolean` |  |
| `alternative_index.revisions[].annual_rate_pct` | `decimal` |  |
| `alternative_index.revisions[].payment` | `decimal` |  |
| `alternative_index.total_paid` | `decimal` |  |
| `alternative_index.total_interest` | `decimal` |  |
| `differences` | `array` |  |
| `differences[].period` | `integer` |  |
| `differences[].year` | `integer` | sí |
| `differences[].payment_own` | `decimal` |  |
| `differences[].payment_alternative` | `decimal` |  |
| `differences[].difference_month` | `decimal` |  |
| `differences[].difference_year` | `decimal` |  |
| `total_paid` | `object` |  |
| `total_paid.own` | `decimal` |  |
| `total_paid.alternative` | `decimal` |  |
| `total_paid.difference` | `decimal` |  |
| `total_interest` | `object` |  |
| `total_interest.own` | `decimal` |  |
| `total_interest.alternative` | `decimal` |  |
| `total_interest.difference` | `decimal` |  |
| `warnings` | `array` |  |
| `warnings[]` | `string` |  |
| `attribution` | `object` |  |
| `attribution.text` | `string` |  |
| `attribution.data_updated_at` | `string` |  |
| `attribution.not_endorsed` | `boolean` |  |

### `loan add`

El préstamo guardado, tal cual quedó

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `object` |  |
| `loan.name` | `string` |  |
| `loan.type` | `string` |  |
| `loan.start_date` | `string` |  |
| `loan.original_principal` | `string` |  |
| `loan.term_months` | `integer` |  |
| `loan.payment_day` | `integer` |  |
| `loan.initial_rate` | `string` |  |
| `loan.initial_period_months` | `integer` | sí |
| `loan.index` | `string` |  |
| `loan.spread` | `string` |  |
| `loan.spread_unbonified` | `string` | sí |
| `loan.revision_every_months` | `integer` |  |
| `loan.reference_offset_months` | `integer` | sí |
| `loan.floor_rate` | `string` | sí |
| `loan.rate_rounding` | `string` |  |
| `loan.interest_convention` | `string` |  |
| `loan.payment_rounding` | `string` |  |
| `loan.early_repayment_fee_pct` | `string` |  |
| `loan.sync` | `object` | sí |
| `loan.id` | `string` |  |
| `loan.complete` | `boolean` |  |
| `loan.missing_fields` | `array` | sí |
| `loan.missing_fields[]` | `string` |  |
| `loan.warnings` | `array` | sí |
| `loan.warnings[]` | `string` |  |

### `loan edit`

El préstamo tras la edición

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `object` |  |
| `loan.name` | `string` |  |
| `loan.type` | `string` |  |
| `loan.start_date` | `string` |  |
| `loan.original_principal` | `string` |  |
| `loan.term_months` | `integer` |  |
| `loan.payment_day` | `integer` |  |
| `loan.initial_rate` | `string` |  |
| `loan.initial_period_months` | `integer` | sí |
| `loan.index` | `string` |  |
| `loan.spread` | `string` |  |
| `loan.spread_unbonified` | `string` | sí |
| `loan.revision_every_months` | `integer` |  |
| `loan.reference_offset_months` | `integer` | sí |
| `loan.floor_rate` | `string` | sí |
| `loan.rate_rounding` | `string` |  |
| `loan.interest_convention` | `string` |  |
| `loan.payment_rounding` | `string` |  |
| `loan.early_repayment_fee_pct` | `string` |  |
| `loan.sync` | `object` | sí |
| `loan.id` | `string` |  |
| `loan.complete` | `boolean` |  |
| `loan.missing_fields` | `array` | sí |
| `loan.missing_fields[]` | `string` |  |
| `loan.warnings` | `array` | sí |
| `loan.warnings[]` | `string` |  |

### `loan sync`

El préstamo tras guardar el recibo

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `object` |  |
| `loan.name` | `string` |  |
| `loan.type` | `string` |  |
| `loan.start_date` | `string` |  |
| `loan.original_principal` | `string` |  |
| `loan.term_months` | `integer` |  |
| `loan.payment_day` | `integer` |  |
| `loan.initial_rate` | `string` |  |
| `loan.initial_period_months` | `integer` | sí |
| `loan.index` | `string` |  |
| `loan.spread` | `string` |  |
| `loan.spread_unbonified` | `string` | sí |
| `loan.revision_every_months` | `integer` |  |
| `loan.reference_offset_months` | `integer` | sí |
| `loan.floor_rate` | `string` | sí |
| `loan.rate_rounding` | `string` |  |
| `loan.interest_convention` | `string` |  |
| `loan.payment_rounding` | `string` |  |
| `loan.early_repayment_fee_pct` | `string` |  |
| `loan.sync` | `object` | sí |
| `loan.id` | `string` |  |
| `loan.complete` | `boolean` |  |
| `loan.missing_fields` | `array` | sí |
| `loan.missing_fields[]` | `string` |  |
| `loan.warnings` | `array` | sí |
| `loan.warnings[]` | `string` |  |

### `loan list`

Los préstamos guardados con su marca de completo

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loans` | `array` |  |
| `loans[].id` | `string` |  |
| `loans[].name` | `string` |  |
| `loans[].type` | `string` |  |
| `loans[].complete` | `boolean` |  |

### `loan show`

Un préstamo guardado con lo que le falta

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `object` |  |
| `loan.name` | `string` |  |
| `loan.type` | `string` |  |
| `loan.start_date` | `string` |  |
| `loan.original_principal` | `string` |  |
| `loan.term_months` | `integer` |  |
| `loan.payment_day` | `integer` |  |
| `loan.initial_rate` | `string` |  |
| `loan.initial_period_months` | `integer` | sí |
| `loan.index` | `string` |  |
| `loan.spread` | `string` |  |
| `loan.spread_unbonified` | `string` | sí |
| `loan.revision_every_months` | `integer` |  |
| `loan.reference_offset_months` | `integer` | sí |
| `loan.floor_rate` | `string` | sí |
| `loan.rate_rounding` | `string` |  |
| `loan.interest_convention` | `string` |  |
| `loan.payment_rounding` | `string` |  |
| `loan.early_repayment_fee_pct` | `string` |  |
| `loan.sync` | `object` | sí |
| `loan.id` | `string` |  |
| `loan.complete` | `boolean` |  |
| `loan.missing_fields` | `array` | sí |
| `loan.missing_fields[]` | `string` |  |
| `loan.warnings` | `array` | sí |
| `loan.warnings[]` | `string` |  |

### `loan remove`

La confirmación del borrado

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `loan` | `string` |  |
| `removed` | `boolean` |  |

### `skills list`

Las recetas embebidas para agentes

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `skills` | `array` |  |
| `skills[].id` | `string` |  |
| `skills[].title` | `string` |  |
| `skills[].summary` | `string` |  |

### `doctor`

El estado de la instalación

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `version` | `string` |  |
| `commit` | `string` |  |
| `config_path` | `string` |  |
| `config_exists` | `boolean` |  |
| `config_mode` | `string` | sí |
| `data_path` | `string` |  |
| `data_exists` | `boolean` |  |
| `data_mode` | `string` | sí |
| `cache_dir` | `string` |  |
| `cache_exists` | `boolean` |  |
| `indices` | `array` |  |
| `indices[].id` | `string` |  |
| `indices[].code` | `string` |  |
| `indices[].name` | `string` |  |
| `loans` | `array` |  |
| `loans[].id` | `string` |  |
| `loans[].name` | `string` | sí |
| `loans[].type` | `string` | sí |
| `loans[].complete` | `boolean` |  |
| `loans[].missing_fields` | `array` | sí |
| `loans[].missing_fields[]` | `string` |  |
| `network` | `object` |  |
| `network.checked` | `boolean` |  |
| `network.ok` | `boolean` |  |
| `network.host` | `string` | sí |
| `network.error` | `string` | sí |
| `warnings` | `array` |  |
| `warnings[]` | `string` |  |

### `config show`

La configuración completa, con los sinks redactados

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `path` | `string` |  |
| `config` | `object` |  |
| `config.schema_version` | `integer` |  |
| `config.indices` | `object` |  |
| `config.loans` | `object` |  |
| `config.sinks` | `object` |  |

### `config get`

El valor de una clave de la config

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `key` | `string` |  |
| `value` | `string` |  |

### `config set`

La clave escrita en la config

| Campo | Tipo | Opcional |
|---|---|---|
| `schema_version` | `integer` |  |
| `key` | `string` |  |
| `value` | `string` |  |

