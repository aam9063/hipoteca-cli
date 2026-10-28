// Package output formats results: JSON by default (P-01), pretty es-ES formatting on
// request.
package output

import (
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/shopspring/decimal"
)

var (
	decimalType   = reflect.TypeOf(decimal.Decimal{})
	rawMsgType    = reflect.TypeOf(json.RawMessage{})
	jsonMarshaler = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshaler = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// WriteJSON writes exactly one JSON value followed by a single final newline.
// One call, one value: a caller that needs JSONL calls it once per line.
//
// Decimal values: any struct containing decimal.Decimal works with no special
// casing at the call site, and every decimal travels as a JSON string with its
// exact decimal representation — never a JSON number, never scientific
// notation, and the scale the value carries preserved ("0.890" stays "0.890").
// This needs its own pass because decimal.Decimal's own json.Marshaler calls
// String(), which trims trailing zeros (NewFromString("3.10") would travel as
// "3.1"), losing scale the source carried (arranque §6.4, decision 3).
// WriteJSON therefore rewrites decimal values into json.RawMessage strings via
// one reflection walk (prepareValue), rebuilding the types of the containers
// that hold decimals; everything else is marshaled untouched by encoding/json.
// A full custom encoder was rejected: re-implementing struct tag semantics
// (omitempty, "-", embedding) is where silent output bugs live.
//
// SetEscapeHTML(false) keeps <, > and & verbatim: the values are user-facing
// text (amounts, messages, attribution), not embedded HTML.
//
// The rewriter is deliberately loud when it cannot do its job: a type that
// contains a decimal.Decimal but cannot be rewritten (it marshals itself, has
// unexported or embedded fields, refers back to itself, or is a map keyed by a
// decimal) makes WriteJSON fail with a *DecimalNotExactError naming the type,
// because the fallback would send the decimal through decimal.Decimal's own
// zero-trimming MarshalJSON. Types without decimals keep falling back to the
// default marshaler silently: nothing there can lose scale.
func WriteJSON(w io.Writer, v any) error {
	tr := newTypeRewrites()
	prepared, err := prepareValue(reflect.ValueOf(v), tr)
	if err != nil {
		// Nothing has been written yet: the value is prepared in full before
		// the encoder runs, so a failure never leaves a half-written JSON value.
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(prepared.Interface())
}

// DecimalNotExactError reports a type that contains a decimal.Decimal but that
// WriteJSON cannot rewrite, so the decimal would reach the output through
// decimal.Decimal's own MarshalJSON. That marshaler trims trailing zeros
// ("3.10" would leave as "3.1"), silently breaking the exact-scale contract
// (arranque §6.4, decision 3): WriteJSON fails instead of sending a decimal
// whose scale the consumer cannot tell apart from an intended one.
type DecimalNotExactError struct {
	// TypeName is the offending type as reflect.Type renders it
	// (e.g. "output.innerSecret").
	TypeName string
	// Path is the struct-field path from the value's root to the offending
	// type, using Go field names (e.g. "row.Inner" when row.Inner holds it);
	// empty when the offending type is the root value itself or the path is
	// not known.
	Path string
}

func (e *DecimalNotExactError) Error() string {
	where := ""
	if e.Path != "" {
		where = fmt.Sprintf(" en el campo %q", e.Path)
	}
	return fmt.Sprintf(
		"el tipo %s contiene un decimal.Decimal que WriteJSON no puede serializar de forma exacta%s: llegaría a la salida por su MarshalJSON propio, que recorta los ceros finales; usa un tipo que el escritor sepa reescribir (struct con campos exportados, slice, array, mapa o puntero que contenga decimal.Decimal)",
		e.TypeName, where)
}

// cannotRewriteDecimal fails when t contains a decimal.Decimal but must be
// left to the default marshaler. Types without decimals fall back silently:
// nothing there can lose scale.
func cannotRewriteDecimal(t reflect.Type, path string) error {
	if containsDecimal(t) {
		return &DecimalNotExactError{TypeName: t.String(), Path: path}
	}
	return nil
}

// containsDecimal reports whether t's type graph reaches decimal.Decimal.
// The visited set stops cycles: every type is inspected once and all its
// fields explored, so a decimal is always found through its first reachable
// field chain.
func containsDecimal(t reflect.Type) bool {
	return containsDecimalIn(t, map[reflect.Type]bool{})
}

func containsDecimalIn(t reflect.Type, visited map[reflect.Type]bool) bool {
	if t == decimalType {
		return true
	}
	if visited[t] {
		return false
	}
	visited[t] = true
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return containsDecimalIn(t.Elem(), visited)
	case reflect.Map:
		return containsDecimalIn(t.Key(), visited) || containsDecimalIn(t.Elem(), visited)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if containsDecimalIn(t.Field(i).Type, visited) {
				return true
			}
		}
	}
	return false
}

// childPath extends a struct-field path with one field name.
func childPath(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}

// rewrite records, per WriteJSON call, what replaces a type: typ is the JSON
// type to marshal instead (json.RawMessage for decimals, a rebuilt type for
// containers over one), and rewrite says whether values of the type must be
// walked at all (an empty interface keeps its type: the concrete value inside
// decides).
type rewrite struct {
	typ     reflect.Type
	rewrite bool
}

// typeRewrites caches rewrites and tracks the types currently being rebuilt,
// so a self-referential type cannot loop forever.
type typeRewrites struct {
	cache  map[reflect.Type]rewrite
	active map[reflect.Type]bool
}

func newTypeRewrites() *typeRewrites {
	return &typeRewrites{
		cache:  map[reflect.Type]rewrite{},
		active: map[reflect.Type]bool{},
	}
}

// rewrittenType returns the rewrite for t, or a *DecimalNotExactError when t
// contains decimals and cannot be rewritten. path is the struct-field path to
// t from the root value, used in that error.
func (tr *typeRewrites) rewrittenType(t reflect.Type, path string) (rewrite, error) {
	if t == decimalType {
		return rewrite{typ: rawMsgType, rewrite: true}, nil
	}
	if r, ok := tr.cache[t]; ok {
		return r, nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		inner, err := tr.rewrittenType(t.Elem(), path)
		if err != nil {
			return rewrite{}, err
		}
		return tr.remember(t, reflect.PointerTo(inner.typ), inner.rewrite), nil
	case reflect.Slice:
		inner, err := tr.rewrittenType(t.Elem(), path)
		if err != nil {
			return rewrite{}, err
		}
		return tr.remember(t, reflect.SliceOf(inner.typ), inner.rewrite), nil
	case reflect.Array:
		inner, err := tr.rewrittenType(t.Elem(), path)
		if err != nil {
			return rewrite{}, err
		}
		return tr.remember(t, reflect.ArrayOf(t.Len(), inner.typ), inner.rewrite), nil
	case reflect.Map:
		// A decimal map key cannot be rewritten either (JSON keys must be
		// strings and map keys must be comparable), and the default marshaler
		// would render it through decimal.Decimal's zero-trimming MarshalText:
		// decimal-bearing maps must be loud, decimal-free maps fall back.
		if t.Key() == decimalType {
			if err := cannotRewriteDecimal(t, path); err != nil {
				return rewrite{}, err
			}
			return tr.remember(t, t, false), nil
		}
		val, err := tr.rewrittenType(t.Elem(), path)
		if err != nil {
			return rewrite{}, err
		}
		return tr.remember(t, reflect.MapOf(t.Key(), val.typ), val.rewrite), nil
	case reflect.Interface:
		if t.NumMethod() != 0 {
			// A non-empty interface has no rebuilt form: leave it untouched.
			return tr.remember(t, t, false), nil
		}
		// The empty interface keeps its type; its concrete value decides.
		return tr.remember(t, t, true), nil
	case reflect.Struct:
		return tr.rewriteStruct(t, path)
	default:
		return tr.remember(t, t, false), nil
	}
}

// rewriteStruct rebuilds t when it holds decimals anywhere in its fields. A
// type that marshals itself (time.Time, decimal.NullDecimal), carries state a
// reflect.StructOf copy cannot rebuild (unexported or embedded fields), or
// refers back to itself while being rebuilt, is left to the default marshaler
// — unless it contains a decimal.Decimal anywhere, in which case the fallback
// would send that decimal through a zero-trimming MarshalJSON and rewriteStruct
// fails with a *DecimalNotExactError instead.
func (tr *typeRewrites) rewriteStruct(t reflect.Type, path string) (rewrite, error) {
	if tr.active[t] {
		return tr.remember(t, t, false), nil
	}
	if t.Implements(jsonMarshaler) || t.Implements(textMarshaler) {
		if err := cannotRewriteDecimal(t, path); err != nil {
			return rewrite{}, err
		}
		return tr.remember(t, t, false), nil
	}
	if hasUnrebuildableFields(t) {
		if err := cannotRewriteDecimal(t, path); err != nil {
			return rewrite{}, err
		}
		return tr.remember(t, t, false), nil
	}
	tr.active[t] = true
	defer delete(tr.active, t)
	fields := make([]reflect.StructField, t.NumField())
	holdsDecimals := false
	for i := range fields {
		src := t.Field(i)
		field, err := tr.rewrittenType(src.Type, childPath(path, src.Name))
		if err != nil {
			return rewrite{}, err
		}
		ft := field.typ
		if !field.rewrite {
			// The field subtree is marshaled untouched: keep its type.
			ft = src.Type
		}
		fields[i] = reflect.StructField{Name: src.Name, Type: ft, Tag: src.Tag}
		holdsDecimals = holdsDecimals || field.rewrite
	}
	if !holdsDecimals {
		return tr.remember(t, t, false), nil
	}
	if tr.cache[t].typ == t {
		// The type referred back to itself mid-build, so the rebuilt copy
		// would still reach the original through the cycle: fail loudly.
		return rewrite{}, &DecimalNotExactError{TypeName: t.String(), Path: path}
	}
	nt := reflect.StructOf(fields)
	return tr.remember(t, nt, true), nil
}

// remember caches the rewrite for t.
func (tr *typeRewrites) remember(t, typ reflect.Type, mustRewrite bool) rewrite {
	r := rewrite{typ: typ, rewrite: mustRewrite}
	tr.cache[t] = r
	return r
}

// hasUnrebuildableFields reports whether t has unexported or embedded fields,
// whose state or promoted methods a reflect.StructOf copy cannot carry over.
func hasUnrebuildableFields(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).PkgPath != "" || t.Field(i).Anonymous {
			return true
		}
	}
	return false
}

// prepareValue returns v with every decimal.Decimal replaced by a
// json.RawMessage carrying the exact decimal string, rebuilding the containers
// that hold one. Values whose type needs no rewrite are returned as they are,
// so the default marshaler keeps full control of them. It fails with the
// *DecimalNotExactError of the first decimal-bearing type that cannot be
// rewritten; the path is not known at value level, so the error carries the
// type name only.
func prepareValue(v reflect.Value, tr *typeRewrites) (reflect.Value, error) {
	if v.Type() == decimalType {
		return reflect.ValueOf(exactDecimal(v.Interface().(decimal.Decimal))), nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return v, nil
		}
		// The concrete value inside decides: a decimal becomes its exact
		// string, anything else round-trips unchanged.
		prepared, err := prepareValue(v.Elem(), tr)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(prepared.Interface()), nil
	}
	r, err := tr.rewrittenType(v.Type(), "")
	if err != nil {
		return reflect.Value{}, err
	}
	if !r.rewrite {
		return v, nil
	}
	switch v.Kind() {
	case reflect.Pointer:
		p := reflect.New(r.typ.Elem())
		elem, err := prepareValue(v.Elem(), tr)
		if err != nil {
			return reflect.Value{}, err
		}
		p.Elem().Set(elem)
		return p, nil
	case reflect.Slice:
		s := reflect.MakeSlice(r.typ, v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			elem, err := prepareValue(v.Index(i), tr)
			if err != nil {
				return reflect.Value{}, err
			}
			s.Index(i).Set(elem)
		}
		return s, nil
	case reflect.Array:
		a := reflect.New(r.typ).Elem()
		for i := 0; i < v.Len(); i++ {
			elem, err := prepareValue(v.Index(i), tr)
			if err != nil {
				return reflect.Value{}, err
			}
			a.Index(i).Set(elem)
		}
		return a, nil
	case reflect.Map:
		m := reflect.MakeMapWithSize(r.typ, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			val, err := prepareValue(iter.Value(), tr)
			if err != nil {
				return reflect.Value{}, err
			}
			m.SetMapIndex(iter.Key(), val)
		}
		return m, nil
	case reflect.Struct:
		s := reflect.New(r.typ).Elem()
		for i := 0; i < v.NumField(); i++ {
			field, err := prepareValue(v.Field(i), tr)
			if err != nil {
				return reflect.Value{}, err
			}
			s.Field(i).Set(field)
		}
		return s, nil
	default:
		return v, nil
	}
}

// exactDecimal renders d keeping the scale the value carries: StringFixed at
// the value's own exponent never changes the value, it only keeps the zeros
// that decimal.String() would trim ("0.890" stays "0.890"). The rendered text
// only ever contains digits, '-' and '.', so it needs no JSON escaping.
func exactDecimal(d decimal.Decimal) json.RawMessage {
	return json.RawMessage(`"` + d.StringFixed(-d.Exponent()) + `"`)
}
