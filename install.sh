#!/bin/sh
# Install xml2pdf from GitHub releases (Linux and macOS).
#
#   curl -fsSL https://raw.githubusercontent.com/sandeepkumarmishra354/xml-to-pdf/main/install.sh | sh
#
# Environment variables:
#   XML2PDF_VERSION      version to install, e.g. 0.1.0 (default: latest release)
#   XML2PDF_INSTALL_DIR  where to put the binary (default: /usr/local/bin,
#                        falling back to ~/.local/bin if it isn't writable)
#
# Binaries fetched this way are not quarantined by macOS, so Gatekeeper
# does not block them the way it blocks browser downloads.
set -eu

REPO="sandeepkumarmishra354/xml-to-pdf"
BINARY="xml2pdf"

info() { printf '%s\n' "$*"; }
fail() { printf 'xml2pdf install: %s\n' "$*" >&2; exit 1; }

have() { command -v "$1" >/dev/null 2>&1; }

# download URL FILE
download() {
	if have curl; then
		curl -fsSL "$1" -o "$2"
	elif have wget; then
		wget -qO "$2" "$1"
	else
		fail "curl or wget is required"
	fi
}

# Resolve the latest release tag from the /releases/latest redirect, which
# avoids GitHub API rate limits.
latest_version() {
	url="https://github.com/$REPO/releases/latest"
	if have curl; then
		loc=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$url")
	else
		# Server headers look like "Location: https://.../tag/v1.2.3 [following]".
		loc=$(wget -S --spider "$url" 2>&1 | sed -n 's/^ *[Ll]ocation: *\([^ ]*\).*/\1/p' | tail -n 1)
	fi
	tag=${loc##*/tag/}
	tag=$(printf '%s' "$tag" | tr -d '\r')
	case "$tag" in
	v[0-9]*) valid_version "${tag#v}" && printf '%s' "${tag#v}" ;;
	*) fail "could not determine the latest release (got '$loc'); set XML2PDF_VERSION" ;;
	esac
}

valid_version() {
	case "$1" in
	'' | *[!0-9A-Za-z.+-]*) fail "invalid version '$1'" ;;
	esac
}

sha256() {
	if have sha256sum; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif have shasum; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "sha256sum or shasum is required to verify the download"
	fi
}

main() {
	case "$(uname -s)" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	MINGW* | MSYS* | CYGWIN*)
		fail "on Windows, download the .zip from https://github.com/$REPO/releases/latest" ;;
	*) fail "unsupported operating system: $(uname -s)" ;;
	esac

	case "$(uname -m)" in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "unsupported architecture: $(uname -m)" ;;
	esac

	version=${XML2PDF_VERSION:-}
	version=${version#v}
	if [ -z "$version" ]; then
		version=$(latest_version)
	fi
	valid_version "$version"

	archive="${BINARY}_${version}_${os}_${arch}.tar.gz"
	base="https://github.com/$REPO/releases/download/v$version"

	tmp=$(mktemp -d 2>/dev/null || mktemp -d -t xml2pdf)
	trap 'rm -rf "$tmp"' EXIT INT TERM

	info "Downloading xml2pdf $version ($os/$arch)..."
	download "$base/$archive" "$tmp/$archive" ||
		fail "download failed: $base/$archive (does version $version exist?)"
	download "$base/checksums.txt" "$tmp/checksums.txt" ||
		fail "download failed: $base/checksums.txt"

	expected=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
	[ -n "$expected" ] || fail "$archive is not listed in checksums.txt"
	actual=$(sha256 "$tmp/$archive")
	[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive (expected $expected, got $actual)"

	tar -xzf "$tmp/$archive" -C "$tmp" "$BINARY" || fail "could not extract $archive"
	chmod +x "$tmp/$BINARY"

	dir=${XML2PDF_INSTALL_DIR:-}
	sudo=""
	if [ -z "$dir" ]; then
		dir=/usr/local/bin
		if [ ! -w "$dir" ]; then
			if have sudo && [ -d "$dir" ]; then
				info "Installing to $dir requires sudo."
				sudo="sudo"
			else
				dir="$HOME/.local/bin"
			fi
		fi
	fi
	$sudo mkdir -p "$dir"
	$sudo mv "$tmp/$BINARY" "$dir/$BINARY"
	if [ "$os" = darwin ] && have xattr; then
		$sudo xattr -d com.apple.quarantine "$dir/$BINARY" 2>/dev/null || true
	fi

	info "Installed $("$dir/$BINARY" --version) to $dir/$BINARY"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) info "Note: $dir is not on your PATH. Add it with: export PATH=\"$dir:\$PATH\"" ;;
	esac
}

main "$@"
