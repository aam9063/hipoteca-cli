# hipoteca-cli

[![ci](https://github.com/aam9063/hipoteca-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/aam9063/hipoteca-cli/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/aam9063/hipoteca-cli)](https://github.com/aam9063/hipoteca-cli/releases)
[![go](https://img.shields.io/badge/go-1.26-00ADD8)](https://go.dev)
[![license](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

Spanish mortgage maths on the command line: **official Banco de España reference indices**
(Euríbor, IRPH, €STR and eleven more), rate revisions, amortization schedules, early
repayment and index comparison. Exact decimals, JSON output, and no financial advice —
by design.

Built to be driven by an AI agent: every command carries its own recipe, and the output
contract is generated from the binary.

```console
$ hipoteca revision casa --date 2027-01
$ hipoteca amortize casa --amount 10000 --mode compare
$ hipoteca compare casa --index irph --spread 1.00
$ hipoteca scenario casa --index-values 1.5,2,2.5,3
$ hipoteca index show euribor-1y --range 60M
```

> **Status:** `v0.1.0` is released. The calculation commands, the agent commands and the
> notification commands are complete and tested on Linux, macOS and Windows, and packaged
> for Homebrew and Scoop.

## Why it exists

Mortgage arithmetic looks simple and is not: the reference month is not the revision month,
the floor applies before the escritura rounding, the monthly interest is rounded to cents
*before* amortizing, and a negative rate shortens the loan instead of stopping it. Getting
one of those wrong costs cents per month and never announces itself.

So this tool does three things a calculator does not:

1. **It refuses to guess.** No escritura data, no answer: `loan_incomplete` names the missing
   field. An index month that the Banco has not published is `index_not_published`, never an
   invented value.
2. **It shows its work.** Every result carries its inputs, the formula applied, and the exact
   Banco attribution with the data update date.
3. **It never recommends.** Zero sentences telling you what to do with your money.

## Installation

**Linux and macOS, one line.** Downloads the binary for your platform, **verifies the
checksum before writing anything**, and installs it:

```bash
curl -fsSL https://raw.githubusercontent.com/aam9063/hipoteca-cli/main/install.sh | sh
```

`HIPOTECA_VERSION=v0.1.0` pins a version; `HIPOTECA_INSTALL_DIR=~/.local/bin` picks the
destination. A checksum that does not match aborts with nothing written and any previous
installation untouched.

**From source.** The only path that does not depend on a release. Needs Go 1.26+:

```bash
git clone https://github.com/aam9063/hipoteca-cli.git
cd hipoteca-cli
make build                       # Windows without make: go build -o bin\hipoteca.exe .\cmd\hipoteca
./bin/hipoteca calc --principal 150000 --rate 3.10 --months 300
```

Upgrades are handled by `hipoteca update` (or `update --check` to only look), which also
verifies the checksum and **leaves the previous installation intact if anything fails**.

**macOS and Linux, Homebrew.** A Cask, because goreleaser v2 no longer generates formulae
for binaries:

```bash
brew tap aam9063/tap
brew install --cask hipoteca
```

**Windows, Scoop:**

```powershell
scoop bucket add aam9063 https://github.com/aam9063/scoop-bucket
scoop install hipoteca
```

## Using it

Register your loan once, from the numbers in your escritura. Nothing is inferred: if you
leave a field out, the tool tells you which one it needs.

```bash
hipoteca loan add casa \
  --name Casa --type variable \
  --start-date 2021-06-01 --principal 180000.00 --months 360 \
  --payment-day 1 --initial-rate 1.25 --initial-period-months 12 \
  --index euribor-1y --spread 0.89 --revision-every-months 12 \
  --reference-offset-months 2 --floor none --rate-rounding none --fee-pct 0
```

Then ask questions:

| Question | Command |
| --- | --- |
| How much will my payment change at the next revision? | `hipoteca revision casa` |
| What happens if I repay €10,000 early? | `hipoteca amortize casa --amount 10000 --mode compare` |
| What if Euríbor goes to 3 %? | `hipoteca scenario casa --index-values 1.5,2,2.5,3` |
| Would IRPH have been cheaper? | `hipoteca compare casa --index irph --spread 1.00` |
| How much is left to pay? | `hipoteca schedule casa` |

Output is one JSON value on stdout. On failure, stdout stays empty and an error envelope
goes to stderr with an exit code: `2` usage or incomplete data · `4` not found or not
published · `5` upstream unavailable · `6` network · `7` the API changed.

## For agents

The binary documents itself, so an agent does not have to guess a flag or read this file:

```bash
hipoteca skills list          # four recipes: id and one-line purpose
hipoteca skills get next-revision  # exact commands, flags, and what to do on each error
hipoteca schema calc          # the output contract, generated from the binary
hipoteca doctor               # version, paths, indices, saved loans and their missing fields
```

`schema` and [`docs/cli-spec.md`](docs/cli-spec.md) are generated from the same structures
the commands write, and a test fails when the committed copy drifts from the binary. A
reference that drifts lies with authority, which is worse than having none.

[`skills/hipoteca/SKILL.md`](skills/hipoteca/SKILL.md) is a ready-made skill for Pi and
Claude Code.

## Guarantees

- **Exact decimals, never floats.** Money and rates are `math/big.Rat` internally and
  `shopspring/decimal` at the API. An AST guard in CI fails on any `float64` in the
  calculation package.
- **No invented data.** A missing escritura field, an unpublished index month or an
  unparseable series is an error with its own code, never a default.
- **No data leaves your machine.** The only network call is the Banco de España. Notifications
  are opt-in, and without `--with-impact` the payload contains no loan data at all.
- **No advice.** No text recommends a decision, and a test enforces it structurally.
- **Verified against a real bank.** One reference case comes from an actual bank receipt:
  a June 2021 settlement whose interest, principal, payment and remaining balance match this
  tool's conventions to the cent.

## How it is validated

Ten reference cases in [`testdata/finance/`](testdata/finance/) are generated outside the Go
code with `numpy-financial`, cross-checked between two independent methods, and fail loudly
if they disagree. The index values in those cases are real published Euríbor months, checked
against captured Banco de España responses in [`testdata/bde/`](testdata/bde/).

The eleventh case is a real bank receipt, transcribed from paper rather than computed. It is
the only thing that proves the conventions match what a bank actually charges.

Every PR runs on Linux, macOS and Windows: `gofmt`, `go mod tidy`, `go vet`, `staticcheck`,
`govulncheck` and the full test suite. No PR test touches the network.

## Data and attribution

Indices come from the Banco de España website. Every output that contains data or
calculations carries an `attribution` block with one of two exact strings and the data update
date:

- Raw data (`index list`, `index show`, `index latest`):
  `Fuente: Sitio web del Banco de España (www.bde.es)`
- Processed data (every calculation):
  `Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)`

`attribution.data_updated_at` is the publication date (`YYYY-MM-DD`) to cite alongside any
figure. Terms of use are summarised in [`docs/series.md`](docs/series.md) §8.

**This project is not affiliated with, sponsored by or endorsed by the Banco de España.** It
does not use their brand or logo and does not imply participation.

## Development

Go 1.26+. `make` is optional; the same commands live in `scripts/`.

```bash
make build     # bin/hipoteca with real version and commit
make check     # gofmt, go mod tidy, go vet, staticcheck, govulncheck and tests
make test      # tests only
```

`make check` runs `scripts/check.sh`, the same script CI runs on three operating systems. It
installs `staticcheck` and `govulncheck` into `./.bin` on first use and leaves your `GOPATH`
alone.

## Documentation

- [`docs/cli-spec.md`](docs/cli-spec.md) — commands and output contract, generated from the binary.
- [`docs/calculos.md`](docs/calculos.md) — every calculation convention, in Spanish.
- [`docs/series.md`](docs/series.md) — index catalogue and API contract.
- [`docs/ARRANQUE.md`](docs/ARRANQUE.md) — project brief and backlog.
- [`AGENTS.md`](AGENTS.md) — how to work in this repository.

## License

MIT. See [`LICENSE`](LICENSE).

The project documentation is written in Spanish; code, comments, commit messages and this
README are in English.
