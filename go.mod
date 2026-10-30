module github.com/aam9063/hipoteca-cli

go 1.26.0

// Toolchain parcheado: go1.26.1 y anteriores llevan vulnerabilidades de stdlib en
// crypto/x509 y net/http que este proyecto hace alcanzables al usar HTTPS (GO-2026-6809,
// GO-2026-6617). `go` declara el mínimo del lenguaje; `toolchain` fija la versión con la que
// se construye y se corre `make check`, y govulncheck falla si vuelve a haber una alcanzable.
toolchain go1.26.10

require (
	github.com/pelletier/go-toml/v2 v2.4.3
	github.com/shopspring/decimal v1.6.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.9
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.61.0
)

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.49.0 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
