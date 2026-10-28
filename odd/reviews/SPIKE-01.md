# Review gate · SPIKE-01

- **Cambio:** `docs/series.md`, `internal/bde/series.yaml`, `testdata/bde/` (13 fixtures),
  `odd/tasks/SPIKE-01-series.md`.
- **Rango revisado:** `dev..a438772` (dos commits: `ccc2e30` documentación, `a438772`
  fixtures).
- **Riesgo declarado:** medio. Sin ítem de alto riesgo, así que no hubo verificador
  independiente (no aplica la regla de `internal/finance`).
- **RDD:** `assess` devolvió `nativeRisk: unassessable` con razón
  `package-local-binary-missing`; el binario nativo no está disponible en este entorno y
  **no hubo review nativo**. Plan devuelto: auto-verificación del autor. Queda anotado como
  limitación, no como aprobación.

## Verificación ejecutada

Un único check reproducible sobre 21 invariantes (YAML parsea, 14 ids y códigos únicos,
campos requeridos, decimales/frecuencia contra la API, ids citados en la documentación,
códigos citados dentro del catálogo, auxiliares, series BOE 404, `simbolo` ausente,
13 fixtures parseables, atribución completa, reglas P-06 y P-07 documentadas):

```
RESULTADO: TODO OK   (21/21)
```

Comprobaciones adicionales hechas por separado:

| Qué | Resultado |
|---|---|
| Orden de `favoritas` = orden de los códigos pedidos | confirmado |
| Día de `fechaValor` siempre `01` | confirmado |
| Hora de `fechaValor` no estable (08:15 / 09:15 en la misma descarga) | confirmado |
| `fechaInicio`/`fechaFin` de la serie completa y no del rango | confirmado |
| Los 14 códigos del catálogo responden (HTTP 200 con forma esperada) | confirmado |
| `listaSeries` con serie o rango inválidos devuelve objeto `errNum` con HTTP 200 | confirmado |
| `favoritas` con serie inexistente devuelve array con `errNum` por elemento | confirmado |
| `idioma` y `series` obligatorios | confirmado |
| Rangos: 30M/60M/MAX/año con datos válidos; 12M/24M/36M/120M y años futuros → 412 | confirmado |
| Mes no publicado ausente de `fechas` (octubre 2026 no está) | confirmado |

## Hallazgos propios (encontrados y corregidos antes de cerrar)

| # | Severidad | Hallazgo | Corrección |
|---|---|---|---|
| 1 | major | Los textos literales de `api_description` de €STR los había escrito a mano; `estr-1w` decía `ESTER TIPOS` y la API dice `ESTER MTIPOS` | Regenerados desde los JSON reales por script, no a ojo |
| 2 | major | Afirmé que las series `D_1TFECHABOE*` "existen": existen en el CSV del Boletín pero **no en la API** (`errNum` 404) | Documentado como no disponible; quitadas del `series.yaml` |
| 3 | major | No documenté que `simbolo` y `tendencia` pueden faltar en una respuesta válida, ni que `decimales` puede ser `0` | Añadido en §1.4 y §3.3 con la evidencia del fixture |
| 4 | minor | "En ráfagas seguidas sí se vieron respuestas fallidas puntuales": no era cierto, los fallos que vi eran de mi propio script | Frase eliminada; queda la recomendación de tratar 429/5xx con backoff |
| 5 | minor | Decía que `euribor-1y` "es el que usan la mayoría de las hipotecas variables en España", sin evidencia | Matizado a observación y aclarado que la CLI no lo asume |
| 6 | minor | Las condiciones de reutilización se presentaban como verificadas por mí | Atribuidas al arranque §2.3, que cita la fuente |
| 7 | nit | `tenor: 3y` para el IRPH, que es "más de 3 años" | `3y+`, con el vocabulario documentado |

## Orden de revisión del arranque §10.5

1. **Corrección:** aplicada (hallazgos 1-3, 7).
2. **P-04, P-05, P-06, P-07:** el documento exige que toda cifra lleve sus entradas (P-04
   es del cálculo, no aplica todavía); la atribución dual con fecha y `not_endorsed` está
   completa (P-05); la regla del mes no publicado prohíbe explícitamente sustituir por el
   último valor y remite a `assumed` (P-06); el documento prohíbe decodificar
   `valor`/`valores` como `float64` y fija `json.Number` → `decimal` (P-07).
3. **Contrato JSON y códigos de salida:** mapeo de errores de la API a los códigos 4/5/7
   del arranque §6.4, incluida la asimetría array/objeto.
4. **Fechas y meses de referencia:** año-mes como única clave; hora descartada; día 01.
5. **Simplicidad:** tres artefactos y un documento; el YAML no repite el CSV, fija lo que
   el binario necesita (`go:embed`).
6. **Tests:** los fixtures son la base de los tests de BDE-01; ningún test de PR toca la
   red.
7. **Rendimiento:** no aplica todavía (no hay código ejecutable).

## Veredicto

**PASS con las correcciones aplicadas.** Ningún principio bloqueado. La limitación
conocida es que no hubo ni review nativo (binario ausente) ni verificador independiente
(riesgo sin ítem de alto riesgo): la evidencia es la auto-verificación reproducible de
arriba, que cualquiera puede volver a correr.

## Pendiente para el usuario

- SPIKE-01 se apoya en el catálogo del Boletín Estadístico para llegar a los códigos. Si
  preferís que se confirme además en HERMES (la vía que pide el arranque), decídmelo y lo
  contrapongo antes de OPS-01.
