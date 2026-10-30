---
name: hipoteca
description: "Trigger: mi hipoteca, Euríbor, IRPH, revisión de cuota, amortizar hipoteca, cuadro de amortización, cuánto pago, cuota mensual, hipoteca variable/mixta/fija. Consulta los índices oficiales del Banco de España y calcula revisiones, cuadros y amortizaciones con la CLI hipoteca."
---

# hipoteca

CLI que calcula sobre una hipoteca real con los índices oficiales del Banco de España.
No es un chatbot: es el motor. Los cálculos salen de aquí, nunca de tu cabeza.

## Regla de oro

**Antes de responder cualquier pregunta de hipoteca, descubre el comando:**

```bash
hipoteca skills list          # cuatro recetas con id y resumen
hipoteca skills get <id>      # la receta completa: comandos exactos, flags, errores
```

**No inventes flags ni comandos.** La receta trae la línea exacta a ejecutar. Si dudas de un
campo de la salida, `hipoteca schema <comando>` describe el contrato, y se genera desde el
binario, así que no puede desfasarse.

## Las cuatro recetas

| Pregunta del usuario | id | Comando |
|---|---|---|
| ¿Cuánto me sube la cuota? | `next-revision` | `hipoteca revision <id> [--date YYYY-MM]` |
| ¿Qué pasa si amortizo X? | `early-repayment` | `hipoteca amortize <id> --amount X --mode compare` |
| ¿Y si el índice va al 3 %? | `sensitivity` | `hipoteca scenario <id> --index-values 1.5,2,2.5,3` |
| ¿Otro índice me habría salido mejor? | `index-comparison` | `hipoteca compare <id> --index irph --spread 1.00` |

## Antes de calcular

```bash
hipoteca doctor            # versión, rutas, índices, préstamos guardados
hipoteca loan list         # ¿está el préstamo?
```

Si **no hay préstamo guardado**, dalo de alta con los datos de la escritura del usuario.
`hipoteca loan add <id> --help` lista todos los flags. **Pídelos, no los supongas:** fecha de
inicio, importe, plazo, diferencial, índice, periodicidad de revisión, desplazamiento del mes
de referencia, suelo y redondeo son datos del contrato.

Si un cálculo responde `loan_incomplete`, el campo que falta está en `missing_fields` de
`hipoteca loan show <id>`. Pídeselo al usuario y guárdalo con `hipoteca loan edit <id> --<campo> <valor>`.

## Lo que no se negocia al explicar

- **Nunca inventes un valor de índice.** Si el mes no está publicado la respuesta es
  `index_not_published` (código 4): dile que espere al Banco, o usa `--assume <valor>` sólo si
  **el usuario** da ese valor, y entonces di siempre que es un supuesto (`index.status: "assumed"`).
- **Nunca recomiendes una decisión financiera.** Presenta las cifras; «te conviene» no está en
  el vocabulario del proyecto.
- **Cita la atribución** que trae cada salida: «Elaboración propia con datos extraídos del
  sitio web del Banco de España (www.bde.es)», con `attribution.data_updated_at`.
- **El banco puede diferir en céntimos** por convenciones de cálculo: manda la escritura.
- Importes y tipos son **cadenas decimales exactas** (`"740.31"`), no números: muéstralas tal cual.

## Contrato de salida

Un único valor JSON en stdout. En un fallo, stdout vacío y un sobre de error en stderr con su
código de salida: 2 uso o dato incompleto · 4 no encontrado o índice no publicado · 5 servicio
de terceros no disponible · 6 red · 7 la API cambió. **No interpretes un fallo como un
resultado vacío.**
