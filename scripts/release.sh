#!/bin/sh
# Build, sign, and package a tmsentinel release into dist/.
#
# Signing uses the Developer ID Application certificate from the login
# keychain. The certificate never leaves the machine: releases are cut
# locally, not in CI (same policy as caskvet).
#
# Usage: scripts/release.sh          # version from cli/cli.go
set -eu

cd "$(dirname "$0")/.."

VERSION=$(sed -n 's/^const Version = "\(.*\)"$/\1/p' cli/cli.go)
if [ -z "$VERSION" ]; then
    echo "cannot determine version from cli/cli.go" >&2
    exit 1
fi

IDENTITY="Developer ID Application: Christian Breiteneder (N8M53QK9TU)"
DIST="dist"
mkdir -p "$DIST"

for arch in arm64 amd64; do
    echo "==> building darwin/$arch"
    GOARCH="$arch" go build -trimpath -ldflags "-s -w" \
        -o "$DIST/tmsentinel" .

    echo "==> signing darwin/$arch"
    codesign --force --options runtime --timestamp \
        --sign "$IDENTITY" "$DIST/tmsentinel"
    codesign --verify --strict "$DIST/tmsentinel"

    echo "==> zipping darwin/$arch"
    rm -f "$DIST/tmsentinel_v${VERSION}_darwin_${arch}.zip"
    # zip from inside dist/ and without --keepParent: keepParent adds the
    # containing directory name (dist/) to the archive path
    ( cd "$DIST" && ditto -c -k tmsentinel \
        "tmsentinel_v${VERSION}_darwin_${arch}.zip" )
done

echo "==> checksums"
rm -f "$DIST/checksums.txt"
( cd "$DIST" && shasum -a 256 tmsentinel_v"${VERSION}"_darwin_*.zip > checksums.txt )

echo
echo "release $VERSION built and signed in $DIST/:"
ls -l "$DIST" | grep -v '^total'
echo
echo "next: scripts/notarize.sh, then gh release create with the zips and checksums.txt"
