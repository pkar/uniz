#!/bin/sh
# uniz installer: downloads the prebuilt binary from the latest GitHub
# release and checks it against that release's checksums.txt, or builds
# from source when no binary fits (then Go is needed). Needs curl. No sudo.
#   curl -fsSL -o install.sh https://raw.githubusercontent.com/pkar/uniz/main/install.sh
#   less install.sh && sh install.sh
# Set UNIZ_INSTALL_DIR to choose the directory.
set -eu

REPO="pkar/uniz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
x86_64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
esac

fail() { echo "uniz install: $*" >&2; exit 1; }

# Resolve the latest release once, then use only that release's URLs.
release=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") || fail "could not resolve latest release"
prefix="https://github.com/$REPO/releases/tag/"
case "$release" in "$prefix"*) tag=${release#"$prefix"} ;; *) fail "invalid release URL" ;; esac
case "$tag" in v[0-9]*) ;; *) fail "invalid release tag" ;; esac
case "$tag" in *[!a-zA-Z0-9.-]*) fail "invalid release tag" ;; esac

asset="uniz-$os-$arch"
base="https://github.com/$REPO/releases/download/$tag"
got=""
if curl -fsSL -o "$tmp/uniz" "$base/$asset" 2>/dev/null; then
	curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "could not download release checksums"
	expected=""
	while read -r digest name extra; do
		if [ "$name" = "$asset" ]; then
			[ -z "$expected" ] && [ -z "$extra" ] || fail "ambiguous checksum entry"
			[ "${#digest}" -eq 64 ] || fail "invalid checksum"
			case "$digest" in *[!0-9a-f]*) fail "invalid checksum" ;; esac
			expected=$digest
		fi
	done < "$tmp/checksums.txt"
	[ -n "$expected" ] || fail "missing checksum for $asset"
	if command -v sha256sum >/dev/null 2>&1; then
		actual=$(sha256sum "$tmp/uniz") || fail "checksum calculation failed"
	elif command -v shasum >/dev/null 2>&1; then
		actual=$(shasum -a 256 "$tmp/uniz") || fail "checksum calculation failed"
	else
		fail "checksum verification requires sha256sum or shasum"
	fi
	[ "${actual%% *}" = "$expected" ] || fail "checksum mismatch for $asset"
	echo "verified prebuilt $asset ($tag)"
	got=1
elif command -v go >/dev/null 2>&1; then
	echo "no prebuilt binary for $os/$arch; building $tag from source..."
	curl -fsSL -o "$tmp/source.tar.gz" "https://github.com/$REPO/archive/refs/tags/$tag.tar.gz" || fail "could not download release source"
	mkdir "$tmp/source"
	tar -xzf "$tmp/source.tar.gz" -C "$tmp/source" --strip-components=1
	(cd "$tmp/source" && go build -trimpath -ldflags "-s -w -X main.version=${tag#v}" -o "$tmp/uniz" ./cmd/uniz)
	got=1
fi
[ -n "$got" ] || fail "no prebuilt binary for $os/$arch and no Go toolchain to build with (https://go.dev/dl)"

BINDIR="${UNIZ_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$BINDIR"
install -m 0755 "$tmp/uniz" "$BINDIR/uniz"

echo "installed $BINDIR/uniz ($("$BINDIR/uniz" --version 2>/dev/null || echo uniz))"
case ":$PATH:" in
*:"$BINDIR":*) ;;
*) echo "note: $BINDIR is not on your PATH" >&2 ;;
esac
