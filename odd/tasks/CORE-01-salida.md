# CORE-01: salida JSON, sobre de error, códigos de salida y atribución

## Objetivo

Implementar el contrato de comunicación de la CLI: un único valor JSON por comando en
stdout, errores con forma estable y código de salida, y el bloque de atribución del Banco
de España. Es lo que convierte la CLI en algo que un agente puede consumir sin adivinar.

## Problema y motivo

Todos los comandos de la Ola 2 en adelante devuelven resultados y pueden fallar. Sin este
contrato cada comando inventaría su propio formato de error y su propio código de salida, y
el agente que lo consuma tendría que distinguir por prosa. Es también lo que hace posible
cumplir P-05 en cada salida sin repetir el texto en veinte sitios.

## Alcance

- Dentro:
  - `internal/output`: escritura de **un** valor JSON a stdout, con los `decimal.Decimal`
    como cadena (`"742.18"`, `"3.101"`) y los meses como entero.
  - `internal/errs`: sobre de error `{"error": {type, message, exit_code, hint}}` y la
    tabla de tipos → códigos del arranque §6.4.
  - `internal/model`: `Attribution` con las dos variantes de P-05, `not_endorsed: true` y
    la fecha de actualización de los datos; `SchemaVersion`.
  - `cmd/hipoteca`: los errores del proceso salen con su código de salida; `Ctrl+C` → 130.
  - Tests e2e: stdout con un único JSON, stdout vacío ante error, códigos 2 y 130,
    atribución presente.
- Fuera:
  - `hipoteca schema` (AGENT-01) y `--pretty` más allá de un formateador mínimo es-ES.
  - Los códigos 4, 5, 6 y 7 en su comportamiento real: dependen de BDE-01 y de LOAN-01.
    Aquí se definen y se prueban **por su tipo**, no por su causa.

## Decisiones (las que un implementador no debe adivinar)

1. **stdout lleva resultados, nunca errores.** Ante un fallo, stdout queda vacío. El sobre
   de error va a **stderr**, en JSON cuando stdout no es un terminal, y en texto legible
   cuando sí lo es (arranque §6.1). Un agente siempre recibe JSON.
2. **Un solo valor JSON y un `\n` final.** Nada de logs ni avisos en stdout: el diagnóstico
   va a stderr. `--pretty` produce texto legible es-ES, no JSON.
3. **Los decimales viajan como cadena.** `decimal.Decimal` se serializa con su
   representación decimal exacta, sin notación científica y sin perder ceros significativos
   cuando el valor los tenga. Nunca como número JSON.
4. **Tabla de tipos, fuente única.** `errs` es el único lugar donde vive el mapeo a códigos:

   | Código | Tipos |
   |---|---|
   | 1 | `internal` |
   | 2 | `usage`, `loan_incomplete`, `cancelled`, `confirmation_required` |
   | 4 | `not_found`, `index_not_published` |
   | 5 | `upstream_unavailable`, `rate_limited` |
   | 6 | `network` |
   | 7 | `api_changed` |
   | 130 | interrumpido por señal (no lleva sobre de error) |

   Un tipo desconocido cae en `internal` (código 1) y **no** inventa un código nuevo.
5. **Atribución con dos textos.** Datos sin procesar: `Fuente: Sitio web del Banco de España
   (www.bde.es)`. Datos procesados: `Elaboración propia con datos extraídos del sitio web
   del Banco de España (www.bde.es)`. En ambos: `not_endorsed: true` y `data_updated_at`.
   El texto se construye en `internal/model`, no en cada comando.
6. **`hint` es opcional en el JSON**, `message` y `type` nunca. El mensaje va en español
   (es texto para el usuario; el código y los identificadores siguen en inglés).

## Superficie de edición (nuestra)

- `internal/output/**`, `internal/errs/**`, `internal/model/**`
- `internal/cli/**`, `cmd/hipoteca/main.go`
- `odd/tasks/CORE-01-salida.md`, `odd/reviews/CORE-01.md`

## Riesgo (nuestra)

**Alto, ítem 3**: el sobre de error y los códigos de salida son el contrato que consumen los
agentes y los scripts. Cambiarlos después rompe a quien ya los usa. Por eso lleva
verificador independiente y los tests e2e no dependen de ningún comando de negocio.

## Checklist

- [ ] CORE-01.1 `output.WriteJSON` con decimales como cadena · Criterio: un `decimal` sale
      `"742.18"` y nunca `742.18`; un solo valor por llamada.
- [ ] CORE-01.2 `errs` con el sobre y la tabla completa de §6.4 · Criterio: los doce tipos
      mapean al código que dice la tabla; un tipo desconocido da 1 y un mensaje explícito.
- [ ] CORE-01.3 `main` sale con el código del error y convierte `Ctrl+C` en 130 ·
      Criterio: e2e con un binario real.
- [ ] CORE-01.4 `model.Attribution` con las dos variantes y `not_endorsed` · Criterio: no
      hay forma de construir una atribución sin `data_updated_at`.
- [ ] CORE-01.5 Tests e2e · Criterio: stdout con un único JSON ante éxito; stdout **vacío**
      y sobre JSON en stderr ante error; código 2 ante uso inválido.
- [ ] CORE-01.6 Verificador independiente · Criterio: informe en `odd/reviews/CORE-01.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Contrato fijado; implementación no iniciada.

## Siguiente paso

Test en rojo, implementación, verificación.

## Log

### L1 · 2026-10-28 · decisión del usuario

Tras cerrar FIN-01, el usuario elige continuar por CORE-01.
