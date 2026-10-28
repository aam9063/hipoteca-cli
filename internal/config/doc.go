// Package config reads and writes the local configuration under XDG paths with 0600
// permissions and atomic writes.
//
// The TOML library is github.com/pelletier/go-toml/v2 (not BurntSushi/toml,
// despite the arranque §8.1 table saying "go-toml/v2"): pelletier/go-toml/v2 is
// the actively maintained v2 line with streaming decode, stable Marshal
// support for maps and struct tags, and it is the library that table's name
// actually refers to. Its availability was confirmed with `go get` (v2.4.3).
package config
