#!/usr/bin/env bash
# Builds release files into dist/.  Used by .github/workflows/release.yml; can be run locally to check a release.
#
#   build-release.sh binary <goos> <goarch> <version>   # dist/duplicacy_<os>_<arch>_<version>
#   build-release.sh deb <version>                       # dist/duplicacy_<version>_amd64.deb (needs dpkg-deb)
#
# <version> is the tag without the leading "v", for example 3.3.0.  macOS binaries need cgo (Keychain support)
# and must be built on macOS; Linux binaries are static (CGO_ENABLED=0).
set -euo pipefail

cd "$(dirname "$0")/../.."
mkdir -p dist
commit=$(git rev-parse --short=8 HEAD)

build_binary() {
	local goos=$1 goarch=$2 version=$3 name cgo
	case "$goos/$goarch" in
	linux/amd64) name=linux_x64 cgo=0 ;;
	darwin/arm64) name=osx_arm64 cgo=1 ;;
	darwin/amd64) name=osx_x64 cgo=1 ;;
	*) echo "unsupported target $goos/$goarch" >&2; exit 1 ;;
	esac
	if [ "$cgo" = 1 ] && [ "$(uname -s)" != Darwin ]; then
		echo "macOS binaries must be built on macOS (Keychain support needs cgo)" >&2
		exit 1
	fi
	local output="dist/duplicacy_${name}_${version}"
	CGO_ENABLED=$cgo GOOS=$goos GOARCH=$goarch go build -trimpath \
		-ldflags "-s -w -X main.Version=${version} -X main.GitCommit=${commit}" -o "$output" ./duplicacy
	echo "$output"
}

build_deb() {
	local version=$1
	local binary="dist/duplicacy_linux_x64_${version}"
	[ -f "$binary" ] || build_binary linux amd64 "$version" > /dev/null
	# Debian sorts "~" before everything, so 3.3.0~rc1 comes before 3.3.0
	local debversion=${version//-/\~}
	local maintainer=${DEB_MAINTAINER:-$(git log -1 --format='%an <%ae>')}
	local root
	root=$(mktemp -d)
	trap 'rm -rf "$root"' RETURN
	chmod 0755 "$root"

	install -D -m 0755 "$binary" "$root/usr/bin/duplicacy"
	install -D -m 0644 LICENSE.md "$root/usr/share/doc/duplicacy/copyright"
	install -D -m 0644 README.md "$root/usr/share/doc/duplicacy/README.md"
	install -D -m 0644 CHANGELOG.md "$root/usr/share/doc/duplicacy/CHANGELOG.md"
	mkdir -p "$root/DEBIAN"
	cat > "$root/DEBIAN/control" <<CONTROL
Package: duplicacy
Version: ${debversion}
Architecture: amd64
Maintainer: ${maintainer}
Section: utils
Priority: optional
Homepage: https://github.com/qmaden/duplicacy
Installed-Size: $(du -sk "$root/usr" | cut -f1)
Description: Lock-free deduplication cloud backup tool
 Duplicacy backs up files to local or cloud storage with cross-computer
 deduplication and no lock server.  This package contains the command line
 client built from the qmaden/duplicacy fork.
CONTROL
	local output="dist/duplicacy_${debversion}_amd64.deb"
	dpkg-deb --root-owner-group --build "$root" "$output" > /dev/null
	echo "$output"
}

case "${1:-}" in
binary) build_binary "$2" "$3" "$4" ;;
deb) build_deb "$2" ;;
*) sed -n '2,8p' "$0"; exit 1 ;;
esac
