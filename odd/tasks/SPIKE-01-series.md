# SPIKE-01: localizar los códigos de serie del Banco de España

## Objetivo

Fijar, con evidencia verificada contra la API real, el código de serie de cada índice
oficial del mercado hipotecario que usa la CLI, y documentar el contrato completo de la
API del Banco de España para que `internal/bde` se pueda implementar sin adivinar nada.

## Problema y motivo

Sin los códigos de serie y sin conocer la forma exacta de las respuestas no se puede
empezar BDE-01 (cliente, caché, `series.yaml`), y sin BDE-01 no hay revisión (FIN-02) ni
`index`/`revision` (CMD-01). Es la dependencia raíz de la Ola 1.

El documento de arranque (§2.2) da por sentado un contrato que en la práctica difiere:
los valores vienen como número JSON, no como cadena; `favoritas` sin `series` no lista
nada; `idioma` es obligatorio; los rangos válidos no son los que sugiere el texto. Todo
esto se ha verificado en vivo y queda documentado aquí y en `docs/series.md`.

## Alcance

- Dentro:
  - Código de serie de los índices de §2.1 del arranque, verificado contra la API.
  - Contrato de `favoritas` y `listaSeries`: parámetros obligatorios, rangos válidos,
    forma del JSON, errores, compresión, orden de los datos.
  - Semántica de "mes no publicado" (base de P-06 y del error `index_not_published`).
  - `internal/bde/series.yaml` con el catálogo compilable.
  - Respuestas reales guardadas en `testdata/bde/` como fixtures, con atribución.
- Fuera:
  - Código Go del cliente (BDE-01), caché y TTL.
  - Decisión sobre series auxiliares del IRPH (`D_1T9H0000A/B/C`) más allá de dejarlas
    documentadas.

## Restricciones

- P-05: atribución exacta en todo artefacto que reproduzca datos del Banco de España.
- P-06: si un mes no está publicado, error `index_not_published`; nunca rellenar.
- P-07: los valores llegan como número JSON; el cliente no debe pasar por `float64`.
- P-01/P-09: `series.yaml` fija identificadores estables (`euribor-1y`, ...), que pasan a
  ser parte del contrato de la CLI.

## Superficie de edición (nuestra)

- `docs/series.md`
- `internal/bde/series.yaml`
- `testdata/bde/**`
- `odd/tasks/SPIKE-01-series.md`

No toca `go.mod`, `Makefile`, CI ni `cmd/`: eso es OPS-01.

## Riesgo (nuestra)

**Medio.** No hay dinero calculado todavía ni código ejecutable, pero `series.yaml` y los
identificadores de índice se vuelven contrato de la CLI (P-09) y los fixtures son la base
de los tests de BDE-01. Un código equivocado produciría cifras falsas más adelante.

## Checklist

- [x] SPIKE-01.1 Verificar los 14 códigos de §2.1 contra la API en vivo · Criterio: cada
      código responde 200 con `serie`, `descripcionCorta`, `codFrecuencia`, `decimales`,
      `simbolo`, `fechaValor`, `valor`.
- [x] SPIKE-01.2 Confirmar el significado de la familia `*572` (€STR) por plazo ·
      Criterio: el plazo de cada código se deduce de `informacion[].[Descripción]` de
      `listaSeries`, no de una suposición por posición.
- [x] SPIKE-01.3 Caracterizar el mes no publicado · Criterio: pedir un rango que incluya
      el mes en curso y comprobar que el mes ausente **no aparece** en `fechas` (no hay
      centinela ni valor nulo).
- [x] SPIKE-01.4 Determinar los rangos válidos de `listaSeries` · Criterio: matriz de
      rangos probada; documentados los aceptados y el mensaje de los rechazados.
- [x] SPIKE-01.5 Documentar el contrato completo en `docs/series.md` · Criterio: quien
      implemente `internal/bde` no necesita volver a tocar la red para la forma base.
- [x] SPIKE-01.6 Fijar `internal/bde/series.yaml` · Criterio: 14 índices con id lógico,
      código, nombre, frecuencia y decimales; coherente 1:1 con `docs/series.md`.
- [x] SPIKE-01.7 Guardar fixtures reales en `testdata/bde/` · Criterio: cubren éxito
      multi-serie, histórico largo, año cerrado, serie corta (fin de serie), y los dos
      errores (serie inexistente, rango inválido); `README.md` con atribución y fecha.
- [ ] SPIKE-01.8 Review gate con informe en `odd/reviews/SPIKE-01.md` · Criterio:
      veredicto sobre P-04/P-05/P-06/P-07, contrato y coherencia docs↔yaml.

## Evidencia de verificación

Descarga del 2026-10-28 contra `https://app.bde.es/bierest/resources/srdatosapp/`.

| Código | Índice | Último valor (2026-09) |
|---|---|---|
| `D_1NBAS972` | Euríbor 1 semana | 2.289 |
| `D_1NBAC972` | Euríbor 1 mes | 2.414 |
| `D_1NBAD972` | Euríbor 3 meses | 2.635 |
| `D_1NBAE972` | Euríbor 6 meses | 2.922 |
| `D_1NBAF472` | Euríbor 1 año | 3.247 |
| `D_1T9H0000` | IRPH conjunto de entidades (vivienda libre, >3 años, España) | 3.189 |
| `D_1T9H0011` | IRS 5 años | 3.450 |
| `D_1T9H0004` | Rendimiento interno deuda pública 2-6 años | 2.983 |
| `D_1T9H0010` | Tipo medio hipotecario 1-5 años (UEM) | 3.600 |
| `D_1NBAS572` | €STR 1 semana | 2.440 |
| `D_1NBAC572` | €STR 1 mes | 2.298 |
| `D_1NBAD572` | €STR 3 meses | 2.231 |
| `D_1NBAE572` | €STR 6 meses | 2.107 |
| `D_1NBAF572` | €STR 1 año | 2.034 |

Hallazgos que corrigen o completan el arranque §2.2:

1. `valor` y `valores` son **números JSON**, no cadenas.
2. `idioma` es **obligatorio** (`es`|`en`); sin él, 412 "Los idiomas disponibles son: es en".
3. `favoritas` sin `series` devuelve 412; nunca devuelve un listado completo.
4. Rangos válidos: `30M`, `60M`, `MAX` y un **año con datos**. `12M`, `24M`, `36M`,
   `120M` y años futuros devuelven 412. `rango` vacío equivale a `30M`.
5. `30M` devuelve 31 puntos y `60M` 61: el rango incluye el mes en curso.
6. `fechas` y `valores` vienen en **orden descendente**.
7. `fechaValor` incluye una hora que **varía** según el mes (08:15, 09:15): solo el
   año-mes es significativo.
8. Un mes no publicado **no aparece** en `fechas`; no hay centinela. Es la base exacta de
   `index_not_published`.
9. Errores: `listaSeries` con serie inexistente o rango inválido devuelve **200 OK con un
   objeto** `{"errNum":412,...}`; `favoritas` con serie inexistente devuelve **200 OK con
   un array** que lleva `{"codigo":..., "errNum":404}`. Hay que mirar la forma del cuerpo,
   no el status HTTP.
10. Todo va **gzip** (`content-encoding: gzip`, `vary: Accept-Encoding`), sin cabeceras de
    rate limit ni `Retry-After` a la vista.
11. Los códigos `*572` (€STR) comparten `descripcionCorta`; el plazo solo se distingue en
    `informacion[].[Descripción]` de `listaSeries`. El mapa es `S`=1 semana, `C`=1 mes,
    `D`=3 meses, `E`=6 meses, `F`=1 año.
12. `simbolo` y `tendencia` **pueden faltar** en una respuesta válida (`D_1T9H0000B` no
    trae `simbolo`), y `decimales` puede ser `0`: no todas las series son porcentajes.
13. Las series de fecha de publicación en el BOE (`D_1TFECHABOE*`) aparecen en el CSV del
    Boletín Estadístico pero **no existen en la API** (`errNum` 404). No se pueden usar
    para datar la publicación.

## Review gate

Ver `odd/reviews/SPIKE-01.md`.

## Progreso

Verificado y documentado; falta el review gate con informe escrito (SPIKE-01.8).

## Siguiente paso

Review gate → PR → merge a `dev`. Después, OPS-01.

## Log

### L1 · 2026-10-28 · pedido del usuario

> Lee bien el md de arranque para que sepas que hay que desarrollar. Antes de nada te voy
> a indicar como vamos a funcionar en git. Tenemos la rama main y de ahi sacas dev y de dev
> salen todas las features, fix etc. Tu vas a commitear pushear y crear la pr y mergearla
> contra dev. Manten siempre buena higiene en git para que no hayan conflictos, ramas out
> of date y demás. Supoongo que el lenguaje en el que tenemos que hacer este proyecto será
> en go, no sé si lo indica en el documento

### L2 · 2026-10-28 · decisión del usuario

Arranque: "SPIKE-01 primero, completo". RDD: "Activo (como dice el doc)".

### L3 · 2026-10-28 · nota de gobierno

El arranque §10.6 dice "merge siempre humano (H-02)". El usuario define otro flujo:
`main` → `dev` → `hc/<id>-descripcion`, y el agente commitea, pushea, abre el PR y
mergea **contra `dev`**. Merge contra `main` sigue siendo del usuario. Prevalece esta
instrucción.
