# AGENT-02 · Referencia al día con el binario

## Qué se hizo

- **`docs/cli-spec.md` generado** (39 KB): `internal/cli/clispec.go` recorre el árbol real de
  cobra, con las banderas en el orden lexicográfico de `pflag`, y vuelca el mismo contrato de
  salida que `hipoteca schema` (misma `schemaTable`, misma tabla de códigos de `internal/errs`).
  Se produce con `go run ./scripts/generate-cli-spec`.
- **Test de no-desfase**: si el documento commiteado no coincide con lo que genera el binario,
  falla.
- **Markas de versión** (S5): `watch` y todo su subárbol aparecen como `v0.2.0 (previsto:
  todavía no disponible)`, heredado a los subcomandos. Nunca se documenta como disponible lo
  que no lo está.
- **README reescrito**: el estado real (los cálculos existen y están contrastados con un
  recibo bancario), arranque rápido desde el código fuente, los cuatro límites que no se
  negocian, y la atribución exacta del Banco de España con su fecha de actualización.

## El test de deriva, verificado a mano

No me fié del informe: edité un carácter de `docs/cli-spec.md` y

```
--- FAIL: TestDocsCLISpecMatchesBinary
    docs/cli-spec.md está desfasado respecto del binario (primera diferencia en la línea 37)
```

Al regenerar, verde y bytes idénticos. Un documento escrito a mano no puede colarse.

## Un no-determinismo que apareció fuera del generador

El texto de ayuda de `--range` incluía el año en curso (`time.Now().Year()`), así que el
documento habría derivado **cada enero** y el test de deriva habría fallado solo, una vez al
año, sin que nadie entendiera por qué. Ahora el texto es estático («entre 1990 y el año en
curso») y `parseRange` no cambió: sigue aceptando años reales.

Es el tipo de defecto que S6 pedía evitar, y apareció en el sitio menos obvio: no en el
generador, sino en un texto de ayuda heredado de CMD-01.

## La dependencia

Enumerar banderas reales exige nombrar `pflag.Flag`, y cobra no lo reexporta. El worker paró
en la frontera de sus superficies en lugar de tocar `go.mod` sin permiso, y preguntó. Se
autorizó la promoción de `pflag v1.0.9` de indirecta a directa: **una línea movida, misma
versión, `go.sum` sin cambios, ningún módulo nuevo**.

Se descartó la alternativa de reflexionar sobre `pflag.Flag` para ahorrarse el import: es la
misma maquinaria astuta que el proyecto ya revirtió dos veces (el rewriter reflexivo de
decimales de CORE-01 y el parser YAML artesanal que se cambió por `yaml.v3`). Un import
honesto cuesta menos que un truco que hay que mantener.

## Verificación

- `go mod tidy` → sin diff.
- `git diff go.mod` → exactamente las dos líneas de la promoción.
- `bash scripts/check.sh` → **EXIT=0**.
- `go run ./scripts/generate-cli-spec` dos veces → byte-idéntico.
- Arranque rápido aislado (`HOME` y `APPDATA` propios): `hipoteca calc --principal 150000
  --rate 3.10 --months 300` → JSON correcto con `attribution`, exit 0.

## Honestidad del README

Dice «desarrollo activo hacia `v0.1.0`» y avisa explícitamente de que **no hay instalador,
`install.sh` ni tap hasta REL-01**; el arranque rápido compila desde el fuente. No promete
nada que no exista.

## Desfase que corregí fuera de las superficies del worker

`docs/ARRANQUE.md` §14 seguía describiendo `AGENT-01` como dependiente de «Olas 2 a 4», que
contradice la decisión de corte, y no decía qué estaba cerrado. Añadí una nota de estado con
la advertencia de que `v0.1.0` no sale de `dev` tal cual, porque el último commit sin avisos
es `1ff3a45`.

## Límite declarado

El documento describe las banderas y el contrato de salida, **no el comportamiento**: no dice
qué pasa si falta el índice, ni qué significa cada código en cada comando. Eso vive en
`skills get`, que es lo que un agente debe leer; la referencia es para humanos que buscan un
flag.
