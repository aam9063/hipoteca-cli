[![ci](https://github.com/aam9063/hipoteca-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/aam9063/hipoteca-cli/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/aam9063/hipoteca-cli)](https://github.com/aam9063/hipoteca-cli/releases)
[![go](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

# hipoteca-cli

![demo](assets/demo.gif)

Spanish mortgage maths on the terminal: official Banco de España indices, rate revisions,
amortization schedules, early repayment and index comparison. Exact decimals, JSON for
agents, tables for people, and no financial advice — by design.

```bash
hipoteca revision casa --date 2022-06 --pretty
```
```
  CONCEPTO              VALOR
  Préstamo              casa
  Fecha de revisión     2022-06-01
  Índice                euribor-1y
  Mes de referencia     2022-04
  Valor del índice      0,013 %
  TIN anterior          1,25 %
  Diferencial           0,89 %
  TIN nuevo             0,903 %
  Capital pendiente     175023,35 €
  Meses restantes       348
  Cuota anterior        599,85 €
  Cuota nueva           571,85 €
  Diferencia al mes     -28,00 €
  Diferencia al año     -336,00 €
  Mes de referencia del índice: 2022-04
  Elaboración propia con datos extraídos del sitio web del Banco de
  España (www.bde.es) (datos actualizados: 2026-09-01)
```

- **Rate revisions** with the index month your escritura specifies, not the month you ask about:
  a revision on 2022-06 applies the 2022-04 Euríbor, because that was the last one published.
- **Amortization schedules** to the cent, with the monthly interest rounded *before* amortizing.
- **Early repayment**, both ways, compared side by side:

```bash
hipoteca amortize --principal 150000 --rate 3.10 --months 300 \
  --amount 10000 --date 60 --mode compare --pretty
```
```
  CONCEPTO                REDUCIR CUOTA            REDUCIR PLAZO
  Importe amortizado      10000,00 €               10000,00 €
  Comisión                0,00 €                   0,00 €
  Cuota antes             719,14 €                 719,14 €
  Cuota después           663,18 €                 719,14 €
  Plazo después (meses)   300                      275
  Intereses ahorrados     3431,18 €                8004,38 €
  Meses ahorrados         0                        25
  Modo: compare · Intereses ahorrados: 3431,18 € reduciendo cuota /
  8004,38 € reduciendo plazo
```

- **Index comparison** with real monthly histories: what your loan would have paid on IRPH
  instead of Euríbor, month by month, up to the last month both published.
- **Scenarios** with index values you choose, always marked as assumptions.
- **Notifications** when the Banco publishes a new month of an index you watch.
- **Made for scripts and AI agents:** JSON by default, stable exit codes, machine-readable
  errors, and the usage guide built into the binary (`hipoteca skills list`).

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/aam9063/hipoteca-cli/main/install.sh | sh
hipoteca doctor
```

macOS and Linux with Homebrew (a Cask — goreleaser no longer generates formulae for binaries):

```bash
brew tap aam9063/tap
brew install --cask hipoteca
```

Windows with Scoop:

```powershell
scoop bucket add aam9063 https://github.com/aam9063/scoop-bucket
scoop install hipoteca
```

From source: Go 1.26+, then `make build` (Windows without make:
`go build -o bin\hipoteca.exe .\cmd\hipoteca`). Update later with `hipoteca update`.

The installer **verifies the checksum before writing anything**: a mismatch aborts with nothing
installed and any previous binary untouched.

## Quickstart

```bash
# register your loan once, from the numbers in your escritura
hipoteca loan add casa --name Casa --type variable \
  --start-date 2021-06-01 --principal 180000 --months 360 --payment-day 1 \
  --initial-rate 1.25 --initial-period-months 12 --index euribor-1y --spread 0.89 \
  --revision-every-months 12 --reference-offset-months 2 --floor none --rate-rounding none --fee-pct 0

# how much will my payment change?
hipoteca revision casa --date 2022-06 --pretty

# what if I repay 10.000 € now?
hipoteca amortize casa --amount 10000 --date 60 --mode compare --pretty

# what does the next stretch of the schedule look like?
hipoteca schedule casa --from 2026-11 --to 2027-02 --pretty

# which index is published and when?
hipoteca index list --pretty
```

`hipoteca loan add --help` lists every flag. **Nothing is inferred:** if a field from your
escritura is missing, the calculation refuses with `loan_incomplete` and names it.

Output is JSON by default (`| jq`); `--pretty` is for people.

## Documentation

| guide | |
| --- | --- |
| [CLI reference](docs/cli-spec.md) | every command, flag and output field, generated from the binary |
| [Calculation conventions](docs/calculos.md) | reference month, floor, rounding, negative rates (Spanish) |
| [Index catalogue](docs/series.md) | the 14 series, their codes and the API contract (Spanish) |
| [Agent guide](skills/hipoteca/SKILL.md) | the skill for Pi and Claude Code |
| [Project brief](docs/ARRANQUE.md) | backlog and principles (Spanish) |

## Good to know

- **Not financial advice.** It calculates and compares; it never recommends. No sentence in any
  command says what you should do with your money, and a test enforces it structurally.
- **It may differ from your bank by cents.** Conventions are documented in
  [`docs/calculos.md`](docs/calculos.md) and validated against a real bank receipt, but day
  count, rounding and the exact index month come from your escritura. Every output carries its
  inputs so you can check.
- **It refuses to guess.** An unpublished index month is `index_not_published`, never a made-up
  value; a missing escritura field is `loan_incomplete` naming the field.
- **Your data stays on your machine.** The only network call is the Banco de España. Notifications
  are opt-in, and without `--with-impact` their payload carries no loan data at all.
- **It does not connect to your bank** and never asks for credentials.
- **Exact decimals, never floats.** Money and rates are `math/big.Rat` internally; an AST guard
  in CI fails on any `float64` in the calculation package.
- **Not affiliated with the Banco de España.** It uses no brand or logo and implies no
  participation.

## What it does not do

- **No projections.** `scenario` and `--assume` are marked `assumed: true` because they are your
  numbers, not published data. Nothing is forecast.
- **No future revisions past the initial period** without a synchronized receipt. The schedule
  uses the initial rate and says so; `hipoteca loan sync` takes your last receipt's balance and
  rate as the new starting point.
- **No prepayment simulation without the fee from your escritura.** `--fee-pct` exists, but the
  value has to come from you.

## Development

```bash
make build     # bin/hipoteca with real version and commit
make check     # gofmt, go mod tidy, go vet, staticcheck, govulncheck and the full suite
make test      # tests only
```

`make check` runs `scripts/check.sh`, the same script CI runs on Linux, macOS and Windows. No PR
test touches the network: the Banco de España client is tested against captured responses in
`testdata/bde/`, and the maths against reference cases in `testdata/finance/` — ten generated
outside Go with `numpy-financial`, and one transcribed from a real bank receipt.

## License

MIT. See [`LICENSE`](LICENSE).

Project documentation is in Spanish; code, comments, commit messages and this README are in
English.
