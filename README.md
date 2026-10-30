# hipoteca-cli

CLI en Go que lee los **tipos de referencia oficiales del mercado hipotecario** que publica
el Banco de España (Euríbor, IRPH y otros) y hace los cálculos de tu hipoteca: revisión de
cuota, cuadro de amortización, amortización anticipada y escenarios. Todo en JSON, pensado
para que lo use un agente.

> **Estado:** desarrollo activo hacia `v0.1.0`. Los comandos de cálculo están implementados
> y contrastados con un recibo bancario real: la suite de `internal/finance` verifica cada
> caso de [`testdata/finance/`](testdata/finance/) contra la escritura y el recibo que lo
> sostienen. Lo que aún no hay es empaquetado (instalador, `install.sh`, tap): de eso se
> ocupa REL-01, y hasta entonces la instalación es desde el código fuente.

## Qué hace

```bash
hipoteca revision casa --date 2027-01        # cuánto cambia la cuota en la revisión
hipoteca amortize casa --amount 10000 --mode compare
hipoteca scenario casa --index-values 1.5,2,2.5,3
hipoteca compare casa --index irph --vs euribor-1y --vs-spread 1
hipoteca index show euribor-1y --range 60M
```

El árbol completo de comandos y banderas, y el contrato exacto de cada salida JSON, están
en la [referencia de la CLI](docs/cli-spec.md), que se genera desde el binario —así que no
puede desfasar de él— y en el propio binario con `hipoteca schema` y `hipoteca skills list`.

## Instalación

A partir de `v0.1.0` hay tres vías. **Mientras no haya un release publicado, sólo funciona
la tercera** (compilar desde el código fuente): las otras dos descargan binarios de las
releases de GitHub.

**Linux y macOS, script de una línea.** Descarga el binario de tu plataforma, **verifica el
checksum antes de escribir nada** y lo instala:

```bash
curl -fsSL https://raw.githubusercontent.com/aam9063/hipoteca-cli/main/install.sh | sh
```

Variables útiles: `HIPOTECA_VERSION=v0.1.0` para fijar una versión y
`HIPOTECA_INSTALL_DIR=~/.local/bin` para elegir el destino. El script rechaza cualquier
checksum que no cuadre sin tocar la instalación anterior.

**macOS y Linux, Homebrew:**

```bash
brew install aam9063/tap/hipoteca
```

**Windows, Scoop:**

```powershell
scoop bucket add aam9063 https://github.com/aam9063/scoop-bucket
scoop install hipoteca
```

**Desde el código fuente**, que es la única que no depende de un release. Necesitas Go 1.26 o
superior y `bash` (en Windows, el de Git Bash sirve para `make`):

```bash
git clone https://github.com/aam9063/hipoteca-cli.git
cd hipoteca-cli
make build                      # en Windows sin make: go build -o bin/hipoteca.exe ./cmd/hipoteca
./bin/hipoteca calc --principal 150000 --rate 3.10 --months 300
```

Una instalación posterior se actualiza con `hipoteca update` (o `hipoteca update --check`
para sólo ver si hay versión nueva), que también verifica el checksum y **deja intacta la
instalación anterior si algo falla**.

El primer comando de cálculo no necesita red. Para consultar índices publicados sí conecta
con el Banco de España, que es la única conexión que hace:

```bash
./bin/hipoteca index list
```

## Lo que no hace (y no va a hacer)

- **No es asesoramiento financiero.** Calcula y compara; no recomienda. No dice "te
  conviene" en ningún comando.
- **No sustituye a tu banco.** Puede diferir en céntimos por convenciones de cálculo, base
  de días o redondeos. La CLI muestra siempre las entradas y la fórmula para que puedas
  contrastar, y permite resincronizar con el capital pendiente real de tu recibo
  (`hipoteca loan sync`).
- **No se conecta a tu banco** ni pide credenciales.
- **No envía tus datos** de hipoteca a ningún sitio. La única red es la del Banco de España.
- **No lleva un LLM dentro.** Matemática financiera determinista, con decimales exactos:
  nada de float.

## De dónde salen los datos

Los tipos de referencia se descargan del sitio web del Banco de España y cada cálculo se
hace sobre ellos. Toda salida con datos o con cálculos lleva el bloque `attribution`, con
los dos textos exactos de atribución y la fecha de actualización del dato:

- Datos sin procesar (`index list`, `index show`, `index latest`):
  `Fuente: Sitio web del Banco de España (www.bde.es)`
- Datos procesados (todos los cálculos de esta CLI):
  `Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)`

El campo `attribution.data_updated_at` es la fecha de última actualización de los datos tal
y como la publica el Banco, en formato `YYYY-MM-DD`: es la fecha que hay que citar junto a
cualquier cifra de esta CLI. Los términos de uso del Banco se resumen en
[`docs/series.md`](docs/series.md) §8.

**Este proyecto no está afiliado, patrocinado ni respaldado por el Banco de España.** No
usa su marca ni su logotipo, y no sugiere que participe en la herramienta.

## Desarrollo

Go 1.26 o superior. `make` es opcional: los mismos comandos están en `scripts/`.

```bash
make build     # bin/hipoteca con versión y commit reales
make check     # gofmt, go mod tidy, go vet, staticcheck, govulncheck y tests
make test      # solo los tests
```

`make check` ejecuta `scripts/check.sh`, que es lo que corre la CI en Linux, macOS y
Windows. La primera vez instala `staticcheck` y `govulncheck` en `./.bin`; no toca tu
`GOPATH`.

Ningún test de PR toca la red. Los tests del cliente del Banco de España usan las capturas
reales de `testdata/bde/`, y los de cálculo, los casos de referencia de `testdata/finance/`.

## Documentación

- [`docs/ARRANQUE.md`](docs/ARRANQUE.md): documento de arranque del proyecto (backlog y
  principios).
- [`docs/series.md`](docs/series.md): catálogo de índices y contrato de la API.
- [`docs/cli-spec.md`](docs/cli-spec.md): referencia de comandos y contrato de salida,
  generada desde el binario.
- [`AGENTS.md`](AGENTS.md): cómo trabajar en el repositorio.

## Licencia

MIT. Ver [`LICENSE`](LICENSE).
