# hipoteca-cli

CLI en Go que lee los **tipos de referencia oficiales del mercado hipotecario** que publica
el Banco de España (Euríbor, IRPH y otros) y hace los cálculos de tu hipoteca: revisión de
cuota, cuadro de amortización, amortización anticipada y escenarios. Todo en JSON, pensado
para que lo use un agente.

> **Estado: pre-alpha.** El repositorio todavía no tiene comandos de cálculo. Lo que ya
> está cerrado es la investigación que los sostiene: el catálogo de series y el contrato
> de la API en [`docs/series.md`](docs/series.md).

## Qué va a hacer

```bash
hipoteca revision casa --date 2027-01        # cuánto cambia la cuota en la revisión
hipoteca amortize casa --amount 10000 --mode compare
hipoteca scenario casa --index-values 1.5,2,2.5,3
hipoteca compare casa --index irph --vs euribor-1y --vs-spread 1
hipoteca index show euribor-1y --range 60M
```

## Lo que no hace (y no va a hacer)

- **No es asesoramiento financiero.** Calcula y compara; no recomienda. No dice "te
  conviene" en ningún comando.
- **No sustituye a tu banco.** Puede diferir en céntimos por convenciones de cálculo, base
  de días o redondeos. La CLI muestra siempre las entradas y la fórmula para que puedas
  contrastar, y permite resincronizar con el capital pendiente real de tu recibo.
- **No se conecta a tu banco** ni pide credenciales.
- **No envía tus datos** de hipoteca a ningún sitio. La única red es la del Banco de España.
- **No lleva un LLM dentro.** Matemática financiera determinista.

## Atribución

Los datos de índices provienen del sitio web del Banco de España:

- Datos sin procesar: `Fuente: Sitio web del Banco de España (www.bde.es)`
- Datos procesados (todos los cálculos de esta CLI):
  `Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)`

Cada salida con datos incluye esa atribución y la fecha de última actualización. Los
términos de uso del Banco se resumen en [`docs/series.md`](docs/series.md) §8.

**Este proyecto no está afiliado, patrocinado ni respaldado por el Banco de España.** No
usa su marca ni su logotipo, y no sugiere que participe en la herramienta.

## Desarrollo

Go 1.25 o superior. `make` es opcional: los mismos comandos están en `scripts/`.

```bash
make build     # bin/hipoteca con versión y commit reales
make check     # gofmt, go mod tidy, go vet, staticcheck, govulncheck y tests
make test      # solo los tests
```

`make check` ejecuta `scripts/check.sh`, que es lo que corre la CI en Linux, macOS y
Windows. La primera vez instala `staticcheck` y `govulncheck` en `./.bin`; no toca tu
`GOPATH`.

Ningún test de PR toca la red. Los tests del cliente del Banco de España usan las capturas
reales de `testdata/bde/`.

## Documentación

- [`docs/ARRANQUE.md`](docs/ARRANQUE.md): documento de arranque del proyecto (backlog y
  principios).
- [`docs/series.md`](docs/series.md): catálogo de índices y contrato de la API.
- [`AGENTS.md`](AGENTS.md): cómo trabajar en el repositorio.

## Licencia

MIT. Ver [`LICENSE`](LICENSE).
