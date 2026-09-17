#!/bin/sh
# install-cloudflared.sh — fetch the cloudflared single binary, verify its
# published SHA-256, and put it somewhere local. No root, no package manager,
# no account, and never a pipe-to-shell (D-124).
#
# Usage:
#   scripts/install-cloudflared.sh --sha256 <hex-or-url> [options]
#
# Options:
#   --sha256 <value>     REQUIRED (or set CLOUDFLARED_SHA256): the SHA-256
#                        Cloudflare publishes for the release asset, as 64 hex
#                        characters or as an http(s) URL serving metadata that
#                        contains it.
#   --version <tag>      release tag, e.g. 2026.9.1
#                        (default: CLOUDFLARED_VERSION, else the latest release)
#   --os <goos>          override the detected OS (linux, darwin, windows)
#   --arch <goarch>      override the detected architecture (amd64, arm64, ...)
#   --dir <path>         install directory (default: ./bin when this is a source
#                        checkout, else ~/.local/bin)
#   --base-url <url>     override the release base URL. The default is
#                        Cloudflare's own GitHub release channel; anything else
#                        is a mirror and is warned about loudly. The checksum
#                        still has to match, which is the real control.
#   -h, --help           this text
#
# Why the checksum is mandatory: the script downloads an executable from the
# internet. Verifying the published digest BEFORE chmod +x is the only step
# that stands between "the presenter installed a tunnel client" and "the
# presenter ran whatever the network served". A mismatch is a hard failure, and
# the download is deleted.
#
# Exit codes: 0 installed, 1 usage/verification failure, 2 missing prerequisite.

set -eu

OFFICIAL_BASE="https://github.com/cloudflare/cloudflared/releases/download"
PROG="install-cloudflared"

usage() {
    sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
}

fail() {
    printf '%s: %s\n' "$PROG" "$1" >&2
    exit "${2:-1}"
}

require() {
    command -v "$1" >/dev/null 2>&1
}

# --- defaults ---------------------------------------------------------------

SHA_ARG="${CLOUDFLARED_SHA256:-}"
VERSION="${CLOUDFLARED_VERSION:-}"
OS_ARG=""
ARCH_ARG=""
DIR_ARG=""
BASE_URL="$OFFICIAL_BASE"

while [ $# -gt 0 ]; do
    case "$1" in
    --sha256)
        [ $# -ge 2 ] || fail "--sha256 needs a value" 1
        SHA_ARG="$2"
        shift 2
        ;;
    --version)
        [ $# -ge 2 ] || fail "--version needs a value" 1
        VERSION="$2"
        shift 2
        ;;
    --os)
        [ $# -ge 2 ] || fail "--os needs a value" 1
        OS_ARG="$2"
        shift 2
        ;;
    --arch)
        [ $# -ge 2 ] || fail "--arch needs a value" 1
        ARCH_ARG="$2"
        shift 2
        ;;
    --dir)
        [ $# -ge 2 ] || fail "--dir needs a value" 1
        DIR_ARG="$2"
        shift 2
        ;;
    --base-url)
        [ $# -ge 2 ] || fail "--base-url needs a value" 1
        BASE_URL="${2%/}"
        shift 2
        ;;
    -h | --help)
        usage
        exit 0
        ;;
    *)
        fail "unknown argument: $1 (try --help)" 1
        ;;
    esac
done

# --- platform ---------------------------------------------------------------

detect_os() {
    if [ -n "$OS_ARG" ]; then
        printf '%s' "$OS_ARG"
        return
    fi
    uname_s=$(uname -s 2>/dev/null || printf '')
    case "$uname_s" in
    Linux) printf 'linux' ;;
    Darwin) printf 'darwin' ;;
    *) printf '' ;;
    esac
}

detect_arch() {
    if [ -n "$ARCH_ARG" ]; then
        printf '%s' "$ARCH_ARG"
        return
    fi
    uname_m=$(uname -m 2>/dev/null || printf '')
    case "$uname_m" in
    x86_64 | amd64) printf 'amd64' ;;
    aarch64 | arm64) printf 'arm64' ;;
    armv7l | armv6l) printf 'arm' ;;
    i386 | i486 | i586 | i686) printf '386' ;;
    *) printf '' ;;
    esac
}

OS_NAME=$(detect_os)
ARCH_NAME=$(detect_arch)

if [ -z "$OS_NAME" ]; then
    fail "unsupported operating system '$(uname -s 2>/dev/null || printf unknown)'; pass --os linux|darwin|windows" 1
fi
if [ -z "$ARCH_NAME" ]; then
    fail "unsupported architecture '$(uname -m 2>/dev/null || printf unknown)'; pass --arch amd64|arm64|arm|386" 1
fi

ASSET="cloudflared-$OS_NAME-$ARCH_NAME"
if [ "$OS_NAME" = "windows" ]; then
    ASSET="$ASSET.exe"
fi

# --- prerequisites ----------------------------------------------------------

DOWNLOADER=""
if require curl; then
    DOWNLOADER="curl"
elif require wget; then
    DOWNLOADER="wget"
else
    fail "neither curl nor wget is installed; install one of them and re-run" 2
fi

CHECKER=""
if require sha256sum; then
    CHECKER="sha256sum"
elif require shasum; then
    CHECKER="shasum -a 256"
else
    fail "neither sha256sum nor 'shasum -a 256' is available; install coreutils (or perl) and re-run" 2
fi

fetch() {
    # fetch <url> <destination>
    if [ "$DOWNLOADER" = "curl" ]; then
        case "$1" in
        https://*) curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1" ;;
        file://*) curl -fsSL -o "$2" "$1" ;;
        *) curl -fsSL -o "$2" "$1" ;;
        esac
    else
        case "$1" in
        http://*) fail "refusing a plaintext download from $1; use https" 1 ;;
        *) wget -q -O "$2" "$1" ;;
        esac
    fi
}

# --- the checksum is mandatory ---------------------------------------------

if [ -z "$SHA_ARG" ]; then
    fail "no SHA-256 supplied. Pass --sha256 <64 hex chars> (or an https URL that serves the
       published digest), or set CLOUDFLARED_SHA256. Cloudflare publishes the digest
       beside each release asset at:
         https://github.com/cloudflare/cloudflared/releases
       Nothing is downloaded until the expected digest is known." 1
fi

case "$BASE_URL" in
"$OFFICIAL_BASE"*) : ;;
*)
    printf '%s: WARNING: %s is not Cloudflare'\''s release channel; the download is only as\n' "$PROG" "$BASE_URL" >&2
    printf '%s:          trustworthy as the digest you supplied.\n' "$PROG" >&2
    ;;
esac

TMP_WORK=$(mktemp -d "${TMPDIR:-/tmp}/$PROG.XXXXXX") || fail "cannot create a temporary directory" 2
cleanup() { rm -rf "$TMP_WORK"; }
trap cleanup EXIT HUP INT TERM

# A URL for --sha256 is fetched as published metadata; the digest is the first
# 64-character hex token in it.
case "$SHA_ARG" in
http://* | https://* | file://*)
    META="$TMP_WORK/digest.meta"
    fetch "$SHA_ARG" "$META" || fail "could not download the published digest from $SHA_ARG" 2
    EXPECTED=$(tr -c 'A-Za-z0-9' '\n' <"$META" | grep -E '^[0-9a-fA-F]{64}$' | head -n 1 || true)
    [ -n "$EXPECTED" ] || fail "no SHA-256 digest found in the metadata at $SHA_ARG" 1
    ;;
*)
    EXPECTED="$SHA_ARG"
    ;;
esac

EXPECTED=$(printf '%s' "$EXPECTED" | tr 'A-F' 'a-f')
printf '%s' "$EXPECTED" | grep -Eq '^[0-9a-f]{64}$' ||
    fail "the supplied checksum is not 64 hex characters: $EXPECTED" 1

# --- destination ------------------------------------------------------------

DEST_DIR="$DIR_ARG"
if [ -z "$DEST_DIR" ]; then
    if [ -d ./cmd ] && [ -f ./go.mod ]; then
        DEST_DIR="./bin"
    else
        DEST_DIR="${HOME:-$TMP_WORK}/.local/bin"
    fi
fi
mkdir -p "$DEST_DIR" || fail "cannot create the install directory $DEST_DIR" 2
DEST_DIR=$(cd "$DEST_DIR" && pwd)

if [ -z "$VERSION" ]; then
    URL="$BASE_URL/latest/download/$ASSET"
else
    case "$VERSION" in
    v*) URL="$BASE_URL/$VERSION/$ASSET" ;;
    *) URL="$BASE_URL/v$VERSION/$ASSET" ;;
    esac
fi

# --- download, verify, THEN make it executable ------------------------------

TMP_BIN="$TMP_WORK/$ASSET"
printf '%s: downloading %s\n' "$PROG" "$URL"
fetch "$URL" "$TMP_BIN" || fail "download failed: $URL" 2

ACTUAL=$($CHECKER <"$TMP_BIN" | tr -c 'A-Za-z0-9' '\n' | grep -E '^[0-9a-fA-F]{64}$' | head -n 1 || true)
[ -n "$ACTUAL" ] || fail "could not compute a SHA-256 for the download" 1

if [ "$ACTUAL" != "$EXPECTED" ]; then
    rm -f "$TMP_BIN"
    fail "checksum mismatch — refusing to install.
       expected: $EXPECTED
       actual:   $ACTUAL
       The download was deleted and nothing was made executable." 1
fi

# Verified: only now does the artifact become executable, and it moves into
# place atomically (same filesystem, inside the destination directory).
chmod 0755 "$TMP_BIN" || fail "could not mark the verified download executable" 2
mv -f "$TMP_BIN" "$DEST_DIR/$ASSET" || fail "could not move the binary into $DEST_DIR" 2

printf '%s: installed %s (sha256 %s)\n' "$PROG" "$DEST_DIR/$ASSET" "$EXPECTED"
printf '%s: point the server at it with --cloudflared-bin %s, or set TUNNEL_BIN=%s\n' \
    "$PROG" "$DEST_DIR/$ASSET" "$DEST_DIR/$ASSET"
