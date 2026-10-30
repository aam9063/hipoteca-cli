#!/bin/sh
# Instalador de hipoteca-cli.
#
#   curl -fsSL https://raw.githubusercontent.com/aam9063/hipoteca-cli/main/install.sh | sh
#
# Descarga la release, **verifica el checksum antes de escribir nada** y coloca
# el binario. Un checksum que no cuadra aborta sin tocar la instalación previa.
#
# Variables de entorno:
#   HIPOTECA_VERSION      versión a instalar, p. ej. v0.1.0 (por defecto: la última)
#   HIPOTECA_INSTALL_DIR  destino (por defecto: ~/.local/bin si no existe /usr/local/bin)
#   HIPOTECA_NO_MODIFY_PATH=1  no tocar el PATH ni avisar de ello
#
# Windows no lo usa: allí la vía es Scoop.
set -eu

REPO="aam9063/hipoteca-cli"
# goreleaser nombra los archivos con el nombre del PROYECTO (.ProjectName),
# y el binario de dentro con el del build: son distintos.
PROJECT="hipoteca-cli"
BIN="hipoteca"
DRY_RUN=0

for arg in "$@"; do
	case "$arg" in
	--dry-run) DRY_RUN=1 ;;
	-h | --help)
		sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "argumento desconocido: $arg" >&2
		exit 2
		;;
	esac
done

die() {
	printf 'hipoteca: %s\n' "$1" >&2
	exit "${2:-1}"
}

detect_os() {
	os=$(uname -s)
	case "$os" in
	Linux) echo linux ;;
	Darwin) echo darwin ;;
	*) die "sistema operativo no soportado: $os. En Windows usa Scoop." 2 ;;
	esac
}

detect_arch() {
	arch=$(uname -m)
	case "$arch" in
	x86_64 | amd64) echo amd64 ;;
	arm64 | aarch64) echo arm64 ;;
	*) die "arquitectura no soportada: $arch (se soportan x86_64 y arm64)" 2 ;;
	esac
}

default_dest() {
	if [ -n "${HIPOTECA_INSTALL_DIR:-}" ]; then
		echo "$HIPOTECA_INSTALL_DIR"
	elif [ -w /usr/local/bin ] 2>/dev/null; then
		echo /usr/local/bin
	else
		echo "$HOME/.local/bin"
	fi
}

need() {
	command -v "$1" >/dev/null 2>&1 || die "hace falta $1 y no está en el PATH" 1
}

OS=$(detect_os)
ARCH=$(detect_arch)
DEST_DIR=$(default_dest)
DEST="$DEST_DIR/$BIN"

# La versión se resuelve contra la API porque el nombre del archivo la lleva.
# Con --dry-run no se toca la red: se muestra qué se pediría.
VERSION="${HIPOTECA_VERSION:-}"
if [ -z "$VERSION" ]; then
	if [ "$DRY_RUN" -eq 1 ]; then
		VERSION="<la última>"
	else
		need curl
		VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
			sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)
		[ -n "$VERSION" ] || die "no se pudo averiguar la última versión publicada" 5
	fi
fi

# goreleaser nombra los archivos sin la "v" del tag.
NUM_VERSION=$(printf '%s' "$VERSION" | sed 's/^v//')
EXT="tar.gz"
ASSET="${PROJECT}_${NUM_VERSION}_${OS}_${ARCH}.${EXT}"
BASE="https://github.com/$REPO/releases/download/${VERSION:-latest}"
URL="$BASE/$ASSET"

if [ "$DRY_RUN" -eq 1 ]; then
	cat <<EOF
sistema:     $OS
arquitectura: $ARCH
versión:     $VERSION
archivo:     $ASSET
url:         $URL

destino:     $DEST
EOF
	exit 0
fi

need curl
need tar
if command -v sha256sum >/dev/null 2>&1; then
	SHA_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
	SHA_CMD="shasum -a 256"
else
	die "hace falta sha256sum o shasum para verificar la descarga" 1
fi

TMP=$(mktemp -d)
STAGED=""
cleanup() {
	rm -rf "$TMP"
	if [ -n "$STAGED" ] && [ -f "$STAGED" ]; then rm -f "$STAGED"; fi
}
trap cleanup EXIT INT TERM

printf 'Descargando %s…\n' "$VERSION" >&2
curl -fsSL -o "$TMP/$ASSET" "$URL" || die "no se pudo descargar $URL" 6
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt" || die "no se pudo descargar checksums.txt" 6

# El checksum se verifica ANTES de extraer y de escribir nada en el destino: si
# no cuadra, la instalación anterior queda intacta y el usuario lo sabe.
EXPECTED=$(sed -n "s/^\([0-9a-f]\{64\}\)[[:space:]]\+${ASSET}\$/\1/p" "$TMP/checksums.txt" | head -n1)
[ -n "$EXPECTED" ] || die "checksums.txt no declara ${ASSET}: no se instala nada" 7

ACTUAL=$(cd "$TMP" && $SHA_CMD "$ASSET" | awk '{print $1}')
if [ "$ACTUAL" != "$EXPECTED" ]; then
	die "el checksum no cuadra (esperado $EXPECTED, obtenido $ACTUAL): no se ha instalado nada" 5
fi

tar -xzf "$TMP/$ASSET" -C "$TMP" || die "el archivo de la release está corrupto" 7
[ -f "$TMP/$BIN" ] || die "el archivo de la release no contiene $BIN" 7

# Escritura atómica: se copia al mismo sistema de archivos y se renombra, así
# un corte a mitad no deja un binario a medias.
mkdir -p "$DEST_DIR" || die "no se pudo crear $DEST_DIR" 1
STAGED="$DEST_DIR/.$BIN.new.$$"
cp "$TMP/$BIN" "$STAGED" || die "no se pudo escribir en $DEST_DIR" 1
chmod 0755 "$STAGED"
mv -f "$STAGED" "$DEST" || die "no se pudo mover el binario a $DEST" 1
STAGED=""

printf 'Instalado %s en %s\n' "$("$DEST" version 2>/dev/null | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')" "$DEST" >&2

case ":$PATH:" in
*":$DEST_DIR:"*) ;;
*)
	[ "${HIPOTECA_NO_MODIFY_PATH:-0}" = "1" ] && exit 0
	printf '\n%s no está en el PATH. Añadilo con:\n\n  export PATH="%s:$PATH"\n\n' \
		"$DEST_DIR" "$DEST_DIR" >&2
	;;
esac
