# FIN-04: comparativa de índices con históricos reales

## Objetivo

Responder la pregunta del arranque que todavía no tiene comando: «mi hipoteca es IRPH, ¿qué
habría pagado en los últimos años con Euríbor más 1 %?». Recalcular la historia del préstamo
con **otro índice y otro diferencial**, usando los valores oficiales de cada mes.

## Problema y motivo

Es la comparación que fundamentó miles de reclamaciones por IRPH en España, y ninguna CLI la
ofrece con los índices oficiales. Y no se puede hacer con valores fijos: cada revisión usa el
índice **de su propio mes de referencia**, así que hace falta el histórico real del índice
alternativo, mes a mes.

## Alcance

- Dentro:
  - `hipoteca compare [<id>] --index <otro> --spread <otro> [--from <YYYY-MM>] [--to <YYYY-MM>]`.
  - Recalcular las revisiones del préstamo con el índice alternativo mes a mes y comparar:
    cuota media, total pagado, total de intereses y diferencia por año.
  - `--index-values` para el caso sin préstamo o con datos supuestos, marcados `assumed`.
  - Revisar el advisory de LOAN-01: un préstamo **fijo sin `sync`** debería poder revisarse (su
    cuadro es exacto); el rechazo por «no hay capital» sólo debe aplicar a un variable sin
    histórico de revisiones.
- Fuera:
  - Escenarios de tipo futuro sobre el histórico (ya cubiertos por `scenario` de CMD-02).

## Decisiones

1. **Cada revisión usa el índice de su mes de referencia**, no un valor único: es la diferencia
   entre una comparación real y una estimación. Y la comparación es sobre el pasado: cubre
   hasta la última revisión cuyo mes de referencia está publicado en los dos índices, declara
   esa cobertura en la salida (`coverage`) y avisa cuando el plazo sigue más allá: las
   revisiones posteriores no se estiman ni se proyectan (P-06).
2. **Si falta un mes del índice alternativo, no se inventa**: `index_not_published`/4 con el mes
   concreto. Para una comparativa histórica, interpolar sería fabricar la respuesta.
3. **La salida compara, no recomienda** (P-08): dos bloques simétricos con sus cifras y la
   diferencia, sin ganador ni adjetivos. El test de palabras prohibidas de CMD-02 se extiende.
4. **Ambos índices se declaran en la salida** con su serie y su mes de referencia por año, y la
   atribución de datos procesados con la fecha de la última actualización usada.
5. **El cálculo es el mismo núcleo**: `CompoundSchedule` con los tipos revisados de cada año, no
   un camino paralelo.

## Superficie de edición (nuestra)

- `internal/app/compare.go`, `internal/app/compare_test.go`, `internal/app/revision.go`
- `internal/cli/compare.go`, `internal/cli/compare_test.go`, `internal/cli/root.go`
- `odd/tasks/FIN-04-compare.md`, `odd/reviews/FIN-04.md`

## Riesgo (nuestra)

**Alto**, ítem 6: es dinero sobre históricos reales y el resultado es una comparación que
alguien puede usar para reclamar. Lleva verificador independiente.

## Checklist

- [ ] FIN-04.1 `compare` con histórico real · Criterio: con los fixtures de dos índices,
      recalcula mes a mes y las revisiones usan el índice de su mes de referencia.
- [ ] FIN-04.2 Falta un mes → `index_not_published`/4 nombrando el mes.
- [ ] FIN-04.3 Salida comparativa sin recomendación · Criterio: el test de palabras prohibidas
      pasa y los dos bloques son simétricos.
- [ ] FIN-04.4 Un préstamo fijo sin `sync` puede revisarse; un variable sin historial sigue
      rechazando con el hint de `sync`.
- [ ] FIN-04.5 Verificador independiente · Criterio: informe en `odd/reviews/FIN-04.md`.

## Evidencia de verificación

Se completa al cerrar.

## Progreso

Decisiones fijadas; implementación no iniciada.

## Siguiente paso

Test en rojo, implementación, verificación.

## Log

### L1 · 2026-10-29 · decisión del usuario

«Claro, sigue»: continuar con FIN-04 tras cerrar LOAN-01.

### L2 · 2026-10-29 · decisión del usuario (cierre)

La comparación no puede exigir el índice de meses futuros: se detiene en el último mes
publicado en los dos índices, `--to` posterior a los publicados es usage/2, y la cobertura
se declara con aviso de no proyección (P-06). Un hueco dentro del periodo publicado sigue
siendo index_not_published/4 (decisión 2).
