# AGENT-02 · Referencia al día con el binario

Alcance: los comandos de `v0.1.0`, es decir las Olas 2 a 3 más los comandos de agente
(`skills`, `schema`, `doctor`, `config`). **`watch` queda fuera**: va en `v0.2.0` por la
decisión de corte.

## Specs

- **S1.** `docs/cli-spec.md` se **genera desde el binario**: árbol de comandos y flags reales,
  y el contrato de salida de `hipoteca schema`. No se escribe a mano ni se copia de este
  documento.
- **S2.** Un test falla si el documento generado difiere del que está en el repositorio. Un
  documento que se desfasa del binario miente con autoridad, que es peor que no tenerlo.
- **S3.** El README deja de afirmar lo que ya no es cierto (hoy dice «pre-alpha, todavía no
  tiene comandos de cálculo», y los tiene) e incluye: qué hace, un arranque rápido que
  funcione copiando y pegando, y los cuatro límites que no se negocian —no es asesoramiento,
  puede diferir de tu banco, no se conecta a tu banco, y no envía tus datos.
- **S4.** El README lleva la **atribución exacta del Banco de España** y explica de dónde sale
  cada dato y con qué fecha de actualización.
- **S5.** `docs/cli-spec.md` marca qué comandos pertenecen a `v0.1.0` y cuáles quedan para
  `v0.2.0`, para que nadie documente como disponible lo que todavía no lo está.
- **S6.** El generador corre en `scripts/` sin dependencias nuevas y produce el mismo archivo
  en los tres sistemas operativos (sin rutas absolutas ni orden no determinista).

## Tasks

| ID | Tarea | Specs |
|---|---|---|
| T1 | Generador del árbol de comandos y flags hacia Markdown | S1, S6 |
| T2 | Volcado del contrato de salida (`schema`) dentro del mismo documento | S1, S5 |
| T3 | Test de no-desfase contra el archivo del repositorio | S2 |
| T4 | README: estado real, arranque rápido, atribución y límites | S3, S4 |

## Log

**L1 (pedido del usuario, verbatim):** «sigue lo que dice el documento».

**L2:** alcance fijado a `v0.1.0`; `watch` fuera por la decisión de corte registrada en
`WATCH-02-servicio.md` (L3).
