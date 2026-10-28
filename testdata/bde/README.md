# testdata/bde · respuestas reales de la API del Banco de España

Fixtures de referencia para el cliente `internal/bde`. Son respuestas **reales**, tal como
las sirvió la API, descomprimidas y sin reformatear: si un fixture cambia, deja de ser
evidencia de la realidad.

- **Origen:** `https://app.bde.es/bierest/resources/srdatosapp/`
- **Fecha de descarga:** 2026-10-28
- **Peticiones exactas:** ver la tabla de abajo
- **Codificación:** UTF-8, JSON
- **Datos bajan hasta:** septiembre de 2026

> Fuente: Sitio web del Banco de España (www.bde.es)
>
> Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)
>
> Última actualización de los datos originales: 2026-10-28 (mes de referencia más
> reciente publicado en la descarga: 2026-09).
>
> Este proyecto no está afiliado, patrocinado ni respaldado por el Banco de España.

Los datos se reproducen sin alterar su contenido ni sus metadatos, conforme a los términos
de uso de las estadísticas del Banco de España. No se usan como base de un servicio de
pago. Ver `docs/series.md` §8.

## Fixtures

| Archivo | Petición | Qué prueba |
|---|---|---|
| `favoritas-14-indices.json` | `favoritas?idioma=es&series=<14 códigos>` | Éxito multi-serie; orden de los elementos igual al de la petición; `valor` numérico |
| `listaSeries-euribor-1y-30M.json` | `listaSeries?idioma=es&series=D_1NBAF472&rango=30M` | Forma completa: `informacion[]`, `fechaInicio`/`fechaFin`, 31 puntos (30M incluye el mes en curso) |
| `listaSeries-euribor-1y-MAX.json` | `listaSeries?idioma=es&series=D_1NBAF472&rango=MAX` | Histórico largo (333 puntos, desde 1999-01); orden descendente |
| `listaSeries-euribor-1y-2025.json` | `listaSeries?idioma=es&series=D_1NBAF472&rango=2025` | Año cerrado: 12 puntos |
| `listaSeries-euribor-1y-2026.json` | `listaSeries?idioma=es&series=D_1NBAF472&rango=2026` | **Mes no publicado:** septiembre es el último y octubre no aparece (9 puntos) |
| `listaSeries-estr-1w-30M.json` | `listaSeries?idioma=es&series=D_1NBAS572&rango=30M` | Serie con `descripcionCorta` ambigua: el plazo real solo está en `informacion[]` |
| `listaSeries-irph-30M.json` | `listaSeries?idioma=es&series=D_1T9H0000&rango=30M` | IRPH: mismo contrato que Euríbor |
| `error-listaSeries-serie-inexistente.json` | `listaSeries?idioma=es&series=XX&rango=MAX` | Error como **objeto** con `errNum` 412, HTTP 200 |
| `error-listaSeries-rango-invalido.json` | `listaSeries?idioma=es&series=D_1NBAF472&rango=12M` | Rango no soportado: `errMsgDebug` "El rango 12M no es correcto" |
| `error-listaSeries-sin-idioma.json` | `listaSeries?series=D_1NBAF472&rango=30M` | `idioma` es obligatorio |
| `error-favoritas-serie-inexistente.json` | `favoritas?idioma=es&series=XX` | Error por elemento: **array** con `{"codigo":"XX","errNum":404}` |
| `error-favoritas-sin-series.json` | `favoritas?idioma=es` | `series` es obligatorio |
| `favoritas-series-auxiliares.json` | `favoritas?idioma=es&series=D_1T9H0000A,D_1T9H0000B,D_1T9H0000C,D_1TFECHABOE,D_1TFECHABOE2,D_1TFECHABOE3,D_1TFECHABOE4` | Mezcla de éxito y error por elemento en el mismo array; `D_1T9H0000B` **sin `simbolo`**; los `D_1TFECHABOE*` del boletín devuelven `errNum` 404 en la API |

## Reglas para estos archivos

- No se reformatean ni se reordenan: son capturas.
- No se editan para que pase un test. Si un test discrepa, se para y se pregunta
  (arranque §9).
- No hay tests de PR que toquen la red: los tests de `internal/bde` usan estos fixtures.
- Al renovarlos, se actualiza la fecha de descarga de este README y la de
  `docs/series.md`.
