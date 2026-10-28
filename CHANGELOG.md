# Changelog

El formato sigue [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/) y el proyecto
usa [Versionado Semántico](https://semver.org/lang/es/).

La versión del esquema de salida JSON (`schema_version`) se versiona aparte del binario:
ver `docs/ARRANQUE.md` §3, P-09.

## [Unreleased]

### Añadido

- Esqueleto del repositorio: módulo Go, comando `hipoteca version` en JSON, capas
  declaradas en `internal/`, `make check` portable y CI en Linux, macOS y Windows.
- Catálogo de los 14 índices del Banco de España en `internal/bde/series.yaml`.
- Documentación del contrato real de la API en `docs/series.md`, con 13 respuestas reales
  guardadas en `testdata/bde/`.
