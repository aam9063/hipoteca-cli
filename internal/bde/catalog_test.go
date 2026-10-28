package bde

import (
	"strings"
	"testing"
)

func TestCatalogoEmbebido(t *testing.T) {
	cat, err := LoadCatalog(nil)
	assertNoErr(t, err)
	ids := cat.IDs()
	if len(ids) != 14 {
		t.Fatalf("el catálogo debe resolver 14 ids, tiene %d", len(ids))
	}
	want := []string{
		"euribor-1y", "euribor-6m", "euribor-3m", "euribor-1m", "euribor-1w",
		"irph", "irs-5y", "deuda-publica-2a6", "hipotecario-ue-1a5",
		"estr-1w", "estr-1m", "estr-3m", "estr-6m", "estr-1y",
	}
	for i, id := range want {
		if ids[i] != id {
			t.Fatalf("ids[%d] = %q, se esperaba %q (orden de series.yaml)", i, ids[i], id)
		}
	}
	idx, err := cat.Lookup("euribor-1y")
	assertNoErr(t, err)
	if idx.Code != "D_1NBAF472" || idx.Name != "Euríbor a 1 año" || idx.Decimals != 3 || idx.FirstValue != "1999-01" {
		t.Fatalf("euribor-1y = %+v", idx)
	}
	estr, err := cat.Lookup("estr-1w")
	assertNoErr(t, err)
	if estr.Code != "D_1NBAS572" {
		t.Fatalf("estr-1w code = %q, se esperaba D_1NBAS572", estr.Code)
	}
	if got := cat.EndpointBase(); got != "https://app.bde.es/bierest/resources/srdatosapp/" {
		t.Fatalf("endpoint_base = %q", got)
	}
}

func TestCatalogoLookupDesconocido(t *testing.T) {
	cat, err := LoadCatalog(nil)
	assertNoErr(t, err)
	if _, err := cat.Lookup("indice-inventado"); err == nil {
		t.Fatal("un id desconocido debe dar error")
	} else if !strings.Contains(err.Error(), "indice-inventado") {
		t.Fatalf("el error debe nombrar el id: %s", err.Error())
	}
}

func TestCatalogoOverrideCambiaElCodigo(t *testing.T) {
	cat, err := LoadCatalog(map[string]string{"euribor-1y": "D_CODIGO_NUEVO"})
	assertNoErr(t, err)
	idx, err := cat.Lookup("euribor-1y")
	assertNoErr(t, err)
	if idx.Code != "D_CODIGO_NUEVO" {
		t.Fatalf("el override no cambió el código: %q", idx.Code)
	}
	if !cat.hasCode("D_CODIGO_NUEVO") {
		t.Fatal("el código nuevo debe estar en el catálogo")
	}
	if cat.hasCode("D_1NBAF472") {
		t.Fatal("el código antiguo ya no debe resolver tras el override")
	}
}

func TestCatalogoOverrideDesconocido(t *testing.T) {
	_, err := LoadCatalog(map[string]string{"indice-inventado": "D_X"})
	if err == nil {
		t.Fatal("un id desconocido en la configuración es un error, no un índice nuevo")
	}
	if !strings.Contains(err.Error(), "indice-inventado") {
		t.Fatalf("el error debe nombrar el id: %s", err.Error())
	}
}

// TestCatalogoSeriesAuxiliares: los códigos auxiliares del IRPH salen del
// catálogo (un fallo de uno de ellos sería api_changed), pero los códigos del
// BOE que la API no sirve no están en él.
func TestCatalogoSeriesAuxiliares(t *testing.T) {
	cat, err := LoadCatalog(nil)
	assertNoErr(t, err)
	if !cat.hasCode("D_1T9H0000A") || !cat.hasCode("D_1T9H0000B") || !cat.hasCode("D_1T9H0000C") {
		t.Fatal("las series auxiliares del IRPH deben estar en el catálogo")
	}
	if cat.hasCode("D_1TFECHABOE") {
		t.Fatal("las series de fecha BOE no están disponibles en la API y no deben estar en el catálogo")
	}
}

// yamlMínimo es un catálogo mínimo para probar el parser sin tocar el
// series.yaml embebido (que no es editable en una tarea de parser).
const yamlMínimo = `schema_version: 1
verified_at: "2026-10-28"
endpoint_base: "https://app.bde.es/bierest/resources/srdatosapp/"
language: "es"
indices:
  - id: prueba
    code: D_PRUEBA
    name: "Prueba"
    family: prueba
    tenor: 1y
    frequency: monthly
    decimals: 3
    unit: percent
`

// TestCatalogoClaveDesconocidaSeIgnora: una clave que el binario actual no
// conoce no debe abortar el arranque. El catálogo es un dato que este
// proyecto genera, versiona y embebe, no un contrato con un tercero: un
// campo nuevo en series.yaml tiene que ser ignorado hasta que el código lo
// use. Era justo el caso que el parser a mano rechazaba.
func TestCatalogoClaveDesconocidaSeIgnora(t *testing.T) {
	data := "schema_version: 1\n" +
		"verified_at: \"2026-10-28\"\n" +
		"endpoint_base: \"https://app.bde.es/bierest/resources/srdatosapp/\"\n" +
		"language: \"es\"\n" +
		"clave_futura: lo-que-sea\n" +
		"indices:\n" +
		"  - id: prueba\n" +
		"    code: D_PRUEBA\n" +
		"    name: \"Prueba\"\n" +
		"    family: prueba\n" +
		"    tenor: 1y\n" +
		"    frequency: monthly\n" +
		"    decimals: 3\n" +
		"    unit: percent\n" +
		"    clave_futura_item: ignorada\n"
	cat, err := parseCatalog([]byte(data))
	assertNoErr(t, err)
	idx, err := cat.Lookup("prueba")
	assertNoErr(t, err)
	if idx.Code != "D_PRUEBA" || idx.Decimals != 3 {
		t.Fatalf("la clave desconocida alteró el índice: %+v", idx)
	}
}

// TestCatalogoIdDuplicado: dos índices con el mismo id son un error que
// nombra el id repetido.
func TestCatalogoIdDuplicado(t *testing.T) {
	data := yamlMínimo + "  - id: prueba\n    code: D_OTRO\n"
	_, err := parseCatalog([]byte(data))
	if err == nil {
		t.Fatal("un id duplicado debe dar error")
	}
	if !strings.Contains(err.Error(), "prueba") {
		t.Fatalf("el error debe nombrar el id duplicado: %s", err.Error())
	}
}

// TestCatalogoCodigoDuplicado: dos índices con el mismo código del Banco son
// un error que nombra el código repetido.
func TestCatalogoCodigoDuplicado(t *testing.T) {
	data := yamlMínimo + "  - id: prueba-2\n    code: D_PRUEBA\n"
	_, err := parseCatalog([]byte(data))
	if err == nil {
		t.Fatal("un código duplicado debe dar error")
	}
	if !strings.Contains(err.Error(), "D_PRUEBA") {
		t.Fatalf("el error debe nombrar el código duplicado: %s", err.Error())
	}
}

// TestCatalogoSchemaVersionNoSoportada: una schema_version distinta de la
// soportada es un error, nunca una suposición.
func TestCatalogoSchemaVersionNoSoportada(t *testing.T) {
	data := strings.Replace(yamlMínimo, "schema_version: 1", "schema_version: 99", 1)
	_, err := parseCatalog([]byte(data))
	if err == nil {
		t.Fatal("una schema_version no soportada debe dar error")
	}
	if !strings.Contains(err.Error(), "99") {
		t.Fatalf("el error debe nombrar la versión: %s", err.Error())
	}
}

// TestCatalogoValoresEspeciales: los valores de series.yaml deben soportar
// dos puntos, almohadillas, comillas escapadas y apóstrofes. Son los casos
// que el parser a mano resolvía mal (unquote solo recortaba comillas dobles
// externas) y la razón de usar un parser YAML de verdad.
func TestCatalogoValoresEspeciales(t *testing.T) {
	data := "schema_version: 1\n" +
		"endpoint_base: \"https://app.bde.es/bierest/resources/srdatosapp/\"\n" +
		"language: \"es\"\n" +
		"indices:\n" +
		"  - id: especial\n" +
		"    code: D_ESPECIAL\n" +
		"    name: \"Mercado: tipo \\\"oficial\\\" #1 d'hipoteca\"\n" +
		"    family: especial\n" +
		"    tenor: 1y\n" +
		"    frequency: monthly\n" +
		"    decimals: 3\n" +
		"    unit: percent\n" +
		"    description: 'l''IRPH: serie \"oficial\" #referencia'\n"
	cat, err := parseCatalog([]byte(data))
	assertNoErr(t, err)
	idx, err := cat.Lookup("especial")
	assertNoErr(t, err)
	if idx.Name != "Mercado: tipo \"oficial\" #1 d'hipoteca" {
		t.Fatalf("name con caracteres especiales mal parseado: %q", idx.Name)
	}
	if idx.Description != "l'IRPH: serie \"oficial\" #referencia" {
		t.Fatalf("description con comillas simples escapadas mal parseada: %q", idx.Description)
	}
}
