// Package skills holds the embedded recipe catalogue of `hipoteca skills`
// (AGENT-01, docs/ARRANQUE.md §6.5). The acceptance criterion is concrete: an
// agent with no context must be able to resolve the four §6.5 questions using
// only `skills get`, without reading external docs or guessing a flag, so
// every recipe carries the exact commands, in order, with their flags, and
// what to do when the answer is index_not_published or loan_incomplete.
//
// The recipes are data compiled into the binary: they travel with the version
// that actually implements them and can never point at a flag the binary does
// not have.
package skills

// Summary is one line of `skills list`: the id, a title and a single-sentence
// description of what the recipe solves.
type Summary struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// Skill is a full recipe: the one-line summary plus the Body that
// `skills get` prints as text/plain, the single documented exception to the
// "one JSON value on stdout" rule (arranque §6.1).
type Skill struct {
	ID      string
	Title   string
	Summary string
	Body    string
}

// recipes is the catalogue, in the order of the four §6.5 questions: próxima
// revisión, amortizar, sensibilidad e IRPH frente a Euríbor.
var recipes = []Skill{
	{
		ID:      "next-revision",
		Title:   "Próxima revisión de la cuota",
		Summary: "Cuánto cambia la cuota en la próxima revisión (o en una fecha concreta) del préstamo guardado, con el índice oficial del mes de referencia.",
		Body: `## Receta: próxima revisión de la cuota

Qué resuelve: cuánto cambia la cuota en la próxima revisión (o en la revisión
de una fecha concreta) del préstamo guardado, con el valor oficial del índice
del mes de referencia que marca la escritura.

### 1. Comprueba el préstamo guardado

    hipoteca loan list

- Si no hay ninguno, crea uno antes de calcular. TODOS los valores salen de la
  escritura del usuario: pídelos, no los supongas. Ejemplo de un préstamo
  variable revisable cada año:

      hipoteca loan add casa --name Casa --type variable --start-date 2021-06-01 --principal 180000.00 --months 360 --payment-day 1 --initial-rate 1.25 --initial-period-months 12 --index euribor-1y --spread 0.89 --revision-every-months 12 --reference-offset-months 2 --floor none --rate-rounding none --fee-pct 0

  - '--reference-offset-months' es cuántos meses hacia atrás mira la
    escritura para el mes de índice (0 es un valor válido).
  - '--floor none' declara explícitamente que no hay suelo; si lo hay, su
    valor es otro dato de la escritura.
  - '--rate-rounding' es el redondeo de escritura del tipo: none, 1/8, 1/4
    o 0.05.
  - '--index' debe ser un id del catálogo: 'hipoteca index list' los
    muestra con su último valor.
- Si 'loan list' marca el préstamo como 'complete: false', pasa al paso 2.

### 2. Si el préstamo está incompleto (loan_incomplete, código 2)

Los comandos de cálculo responden 'loan_incomplete' mientras falte un dato
de la escritura. No lo inventes:

    hipoteca loan show <id>

La salida lista 'missing_fields': cada nombre es un campo de la escritura.
Pídeselo al usuario, guárdalo y repite:

    hipoteca loan edit <id> --<campo> <valor>

### 3. Calcula la revisión

    hipoteca revision <id>                # la próxima revisión posterior al mes en curso
    hipoteca revision <id> --date 2027-01 # la revisión de un mes concreto (YYYY-MM)

Con un único préstamo guardado el id es opcional: 'hipoteca revision'.

### 4. Si responde index_not_published (código 4)

El índice del mes de referencia aún no está publicado por el Banco de España.
NUNCA inventes un valor:

- Opción segura: dilo al usuario y vuelve a ejecutar cuando el Banco publique
  el mes.
- Escenario: si el USUARIO te da un valor, calcúlalo con él:

      hipoteca revision <id> --assume 2.35

  El valor de '--assume' es un supuesto del usuario, no un dato del Banco:
  la salida lo marca ('index.status: "assumed"') y lo avisa en 'warnings'.
  Dilo siempre al explicar el resultado.

### 5. Cómo leer la salida

- 'index.reference_month' e 'index.value': mes de índice que marca la
  escritura y valor oficial publicado para ese mes.
- 'rate.new': el TIN nuevo (índice + diferencial, con el suelo y el
  redondeo de escritura ya aplicados); la cadena completa del cálculo está en
  'rate' ('previous', 'spread', 'raw', 'after_floor', 'after_rounding').
- 'payment.previous' y 'payment.new': la cuota antes y después;
  'payment.difference_month' y 'payment.difference_year', el cambio.
- 'inputs.outstanding_principal' e 'inputs.outstanding_source': de dónde
  salió el capital (recibo sincronizado, 'sync+schedule', o cuadro propio,
  'schedule').
- Los importes y los tipos son cadenas decimales exactas ("740.31").

### 6. Al explicar

- Cita: «Elaboración propia con datos extraídos del sitio web del Banco de
  España (www.bde.es)», con la fecha 'attribution.data_updated_at'.
- Presenta las cifras; no digas «te conviene»: no es asesoramiento financiero.
- El banco puede diferir en céntimos por convenciones: manda la escritura.`,
	},
	{
		ID:      "early-repayment",
		Title:   "Amortización anticipada",
		Summary: "Qué pasa si amortizo un importe en un mes dado: cuota menor, menos plazo o las dos cifras juntas para comparar, con la comisión de la escritura.",
		Body: `## Receta: amortización anticipada

Qué resuelve: qué pasa si amortizo X euros tras la cuota N: cuota menor
(reduce-payment), menos plazo (reduce-term) o las dos cifras juntas (compare).
Este cálculo no consulta el Banco de España.

### 1. Con un préstamo guardado

    hipoteca amortize <id> --amount 10000 --date 60 --mode compare

- '--amount': importe a amortizar, en euros (10000 o 10000.00).
- '--date': número de la cuota tras la que se amortiza (1, 2, 3…), contado
  desde el inicio del préstamo. NO es una fecha de calendario.
- '--mode': reduce-payment (misma fecha de fin, cuota menor) |
  reduce-term (misma cuota, menos meses) | compare (las dos cifras).
- La comisión ('early_repayment_fee_pct') y el capital pendiente salen del
  préstamo guardado. Con un único préstamo guardado el id es opcional.
- No uses '--principal', '--rate', '--months' ni '--fee-pct' junto a un id:
  con préstamo guardado esas banderas no se admiten.

Sin préstamo guardado, cálculo suelto con las seis banderas:

    hipoteca amortize --principal 150000 --rate 3.10 --months 300 --amount 10000 --date 60 --mode compare --fee-pct 0.50

'--fee-pct' es la comisión de amortización anticipada en tanto por ciento
del importe: es un dato de la escritura, pídelo; si la escritura no tiene,
usa 0. La comisión se paga aparte y no reduce el capital.

### 2. Si el préstamo está incompleto (loan_incomplete, código 2)

    hipoteca loan show <id>

'missing_fields' nombra el campo de la escritura que falta: pídeselo al
usuario, guárdalo y repite:

    hipoteca loan edit <id> --<campo> <valor>

No lo supongas: mientras falte, los comandos de cálculo siguen respondiendo
loan_incomplete.

### 3. Cómo leer la salida

- 'reduce_payment': misma fecha de fin; 'payment_after' es la cuota nueva
  y 'payment_before' la vigente.
- 'reduce_term': misma cuota; 'term_months_after' son los meses que quedan
  después de amortizar ('term_months_before', antes).
- 'interest_saved' y 'months_saved': ahorro de intereses y de meses frente
  a no amortizar.
- 'fee': comisión a pagar; 'outlay': importe amortizado + comisión.
- 'balance_before_event' y 'balance_after_event': el capital pendiente
  antes y después del evento.
- 'closes_at_event': si aparece, el importe agota el préstamo y lo cierra
  en el mes del evento; después del evento no existe ninguna cuota.
- En '--mode compare' la salida trae los dos bloques ('reduce_payment' y
  'reduce_term'). Presenta las dos opciones con sus cifras y NO recomiendes
  ninguna: no es asesoramiento financiero; decide el usuario.

### 4. Al explicar

- La atribución lleva la fecha de elaboración: este cálculo no toca datos del
  Banco de España.
- El banco puede diferir en céntimos: manda la escritura.`,
	},
	{
		ID:      "sensitivity",
		Title:   "Sensibilidad de la cuota al índice",
		Summary: "Cuota y coste total para varios valores del índice elegidos por el usuario (p. ej. si el Euríbor sube al 3 % o baja al 1,5 %); todos los valores son supuestos.",
		Body: `## Receta: sensibilidad de la cuota al índice

Qué resuelve: la cuota y el coste total para varios valores del índice que el
usuario elige (p. ej. qué pasa si el Euríbor sube al 3 % o baja al 1,5 %).
Todos los valores son supuestos del usuario, no datos del Banco de España.
Este cálculo no consulta la red.

### 1. Con un préstamo guardado

    hipoteca scenario <id> --index-values 1.5,2,2.5,3

- '--index-values': valores del índice en tanto por ciento, separados por
  comas, uno por revisión del préstamo guardado.
- Con un único préstamo guardado el id es opcional.

Sin préstamo guardado, cálculo suelto:

    hipoteca scenario --index-values 1.5,2,2.5,3 --principal 150000 --months 300 --spread 0.80

- '--spread': el diferencial en puntos. '--floor' y '--rounding' son
  opcionales y son datos de la escritura: pídelos, no los supongas.

### 2. Si el préstamo está incompleto (loan_incomplete, código 2)

    hipoteca loan show <id>

'missing_fields' nombra el campo de la escritura que falta: pídeselo al
usuario, guárdalo y repite:

    hipoteca loan edit <id> --<campo> <valor>

### 3. Cómo leer la salida

- 'values[]': una entrada por valor supuesto, con 'index_value',
  'annual_rate_pct' (índice + diferencial, con el suelo y el redondeo de
  escritura aplicados), 'payment', 'total_interest' y 'total_paid'.
- 'inputs': capital, plazo, diferencial, suelo y redondeo con los que se
  calculó (P-04: toda cifra va con sus entradas).
- Cada entrada lleva 'assumed: true' y la salida avisa en 'warnings': son
  escenarios, no datos oficiales del Banco de España. Dilo siempre al
  explicar el resultado.
- Los importes y los tipos son cadenas decimales exactas.

### 4. Al explicar

- Presenta las cifras de cada valor supuesto; no digas «te conviene»: no es
  asesoramiento financiero.
- Recuerda que el banco puede diferir en céntimos: manda la escritura.`,
	},
	{
		ID:      "index-comparison",
		Title:   "IRPH frente a Euríbor (u otro índice)",
		Summary: "Qué habría pagado el préstamo con otro índice y diferencial, mes a mes con los históricos oficiales; es una comparación del pasado, no una proyección.",
		Body: `## Receta: IRPH frente a Euríbor (u otro índice)

Qué resuelve: qué habría pagado el préstamo con otro índice y otro diferencial,
recalculando la historia mes a mes con los valores oficiales de cada revisión.
La comparación es sobre el PASADO: cubre hasta la última revisión cuyo mes de
referencia está publicado en los dos índices, y lo declara en 'coverage'.
Este cálculo SÍ consulta el Banco de España.

### 1. Con un préstamo guardado

    hipoteca compare <id> --index irph --spread 1.00

- '--index': id del índice alternativo (irph, euribor-1y…);
  'hipoteca index list' muestra los disponibles.
- '--spread': diferencial del índice alternativo, en puntos. Para
  «IRPH frente a Euríbor más 1 %»: '--index irph --spread 1.00' compara el
  bloque propio de la escritura con IRPH + 1.
- '--from 2021-06' y '--to 2026-01' (opcionales, YYYY-MM): la ventana de
  revisiones que se muestra y compara.
- Con un único préstamo guardado el id es opcional.

### 2. Si el préstamo está incompleto (loan_incomplete, código 2)

    hipoteca loan show <id>

'missing_fields' nombra el campo de la escritura que falta: pídeselo al
usuario, guárdalo y repite:

    hipoteca loan edit <id> --<campo> <valor>

### 3. Si responde index_not_published (código 4)

La comparación sólo cubre meses publicados en los DOS índices y nunca
interpolan ni inventan valores. Opciones:

- Dilo y deja que cubra lo publicado (la salida lo declara en 'coverage').
- Escenario: si el USUARIO lo pide, sustituye el histórico del índice
  alternativo por valores suyos, uno por revisión comparada, marcados assumed:

      hipoteca compare <id> --index irph --spread 1.00 --index-values 2.0,2.2,2.4

### 4. Cómo leer la salida

- 'own_index' y 'alternative_index': los dos bloques recalculados, con la
  cuota de cada revisión ('revisions[].payment'), el valor del índice de su
  mes de referencia y los totales.
- 'differences[]': alternativa − propia por periodo. Una diferencia POSITIVA
  significa que con el índice alternativo se habría pagado MÁS.
- 'total_paid' y 'total_interest': la diferencia de totales entre los dos
  bloques.
- 'coverage.from' y 'coverage.to': la ventana realmente cubierta; si el
  plazo sigue más allá, la salida lo avisa: el resto no se proyecta.
- Los importes y los tipos son cadenas decimales exactas.

### 5. Al explicar

- Cita: «Elaboración propia con datos extraídos del sitio web del Banco de
  España (www.bde.es)», con la fecha 'attribution.data_updated_at'.
- Presenta las dos cifras y NO recomiendes un índice: no es asesoramiento
  financiero; decide el usuario.
- El banco puede diferir en céntimos: manda la escritura.`,
	},
}

// List devuelve el catálogo para `skills list`, en el orden de las cuatro
// preguntas de §6.5.
func List() []Summary {
	out := make([]Summary, 0, len(recipes))
	for _, r := range recipes {
		out = append(out, Summary{ID: r.ID, Title: r.Title, Summary: r.Summary})
	}
	return out
}

// Get devuelve la receta completa por id. Un id desconocido es ok=false: la
// capa CLI lo convierte en not_found/4 con los ids válidos en el hint.
func Get(id string) (Skill, bool) {
	for _, r := range recipes {
		if r.ID == id {
			return r, true
		}
	}
	return Skill{}, false
}

// IDs devuelve los ids del catálogo, para el hint de un id desconocido.
func IDs() []string {
	out := make([]string, 0, len(recipes))
	for _, r := range recipes {
		out = append(out, r.ID)
	}
	return out
}
