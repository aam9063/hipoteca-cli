# Review gate · CMD-02: comandos `amortize` y `scenario`

## Cambio bajo revisión

- **Tarea:** `odd/tasks/CMD-02-amortize.md`.
- **Rama:** `hc/cmd-02-amortize` (base `dev`).
- **Riesgo declarado:** ítem 3, contrato de salida nuevo sobre dinero ya validado.
- **Verificador independiente:** sí (`independentVerifier: true`).

## Veredicto del verificador: PASS (0 blockers, 0 majors, 2 minors, 2 nits)

Lo que confirmó construyendo el binario:

- **P-08 resistió más allá de la lista de palabras**: `compare` es **estructuralmente simétrico**
  —dos bloques con exactamente las mismas claves—, sin campos de ranking, ganador o
  `saving_preferred`, y el orden es fijo, no por puntuación (reduce-term aparece segundo aun
  cuando ahorra más). El barrido de lenguaje asesor no encontró nada en `amortize`,
  `scenario` ni en los sobres de error.
- **Los números**: casos 9 y 10 al céntimo (663.18 / 3431.18; 719.14 / 8004.38 / 275 meses).
  Y `compare` es **idéntico byte a byte** a los modos individuales: dos caminos que podían
  divergir y no lo hacen.
- **La comisión es neutral en ambos modos**, con el punto medio probado (0.015 → 0.02,
  0.01495 → 0.01) y sin tocar nunca el capital.
- **M1**: `closes_at_event` con el mes, `balance_after_event` 0.00 y ninguna cuota fantasma.
- **`scenario`**: los valores se ecoan con su escala (`1.50`, `-1`) y la cadena de `NewRate`
  se verificó con suelo y los cuatro redondeos.

## Los dos minors y el nit, corregidos

| Hallazgo | Corrección |
|---|---|
| En M1, `payment_after` salía `"0"` y su hermano `"0.00"`: dos escalas para dinero en la misma respuesta | `model.NewMoney` normaliza todo **importe** a dos decimales en el borde; `model.Decimal` sigue intacto para los **tipos**, que llevan su escala (`3.10`, `0.890`) |
| `payment_after` significa dos cosas en M1 (`0.00` en reduce-payment, cuota mantenida en reduce-term) sin explicarlo | Documentado en el doc comment de `closes_at_event` y en el bloque `Long` de `amortize --help`: qué contiene en cada modo y por qué |
| Los mensajes de uso filtraban el prefijo interno `finance:` | Se quita en la traducción a `usage`; el mensaje da el hecho y los números, y el redondeo no soportado enumera los válidos en vez de incrustar la cadena interna |

Añadidos los tres probes del verificador: independencia de la comisión en `compare` con 0/1/2,005 %, M1 en ambos modos, y los dos puntos medios de comisión.

## Verificación del autor

- `bash scripts/check.sh` → **EXIT=0**.
- Binario: M1 reduce-payment → `payment_after "0.00"`, `balance_after_event "0.00"`,
  `closes_at_event` presente, `term_months_after 60`. Error de importe excesivo →
  «el importe supera el capital pendiente en ese mes: importe 999999, capital pendiente
  128506.54 en el mes 60», **cero ocurrencias del prefijo interno**.
- Casos 9 y 10 exactos, cero lenguaje de recomendación, `1.50` con su escala y `assumed`.

## Límites declarados

- **`compare` no está expuesto**: depende de FIN-04 (históricos reales por índice). Un comando
  que no puede cumplir su nombre es peor que uno ausente. Se añadirá con FIN-04.
- `--date` es el **índice de cuota**, no una fecha de calendario: sin préstamo guardado no hay
  calendario que indexar. Está en el `--help`.
- `model.NewMoney` usa `Round(2)` (half-up lejos de cero): para importes que ya vienen en
  céntimos exactos no cambia ningún valor, sólo la escala.
- H-03 (recibos reales) sigue abierto.

## Veredicto

**PASS tras la ronda única de correcciones.** La promesa de «no recomendar» está sostenida por
una prueba estructural, no por una lista de palabras, y el contrato de dinero tiene una sola
escala.
