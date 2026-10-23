#!/bin/sh
# Notarize the release zips in dist/ (or the directory given as $1).
#
# Requires an App Store Connect API key stored once in the login
# keychain (profile shared with caskvet):
#
#   xcrun notarytool store-credentials caskvet-notary \
#     --key AuthKey_XXXXXXXXXX.p8 --key-id XXXXXXXXXXXX \
#     --issuer 00000000-0000-0000-0000-000000000000
#
# Run after scripts/release.sh. Notarization is server-side: the zip
# bytes do not change, so no re-upload is needed.
set -eu

cd "$(dirname "$0")/.."

PROFILE="caskvet-notary"
DIST="${1:-dist}"

set -- "$DIST"/*.zip
if [ ! -e "$1" ]; then
    echo "no zips found in $DIST" >&2
    exit 1
fi

for zip in "$@"; do
    echo "notarizing $zip ..."
    xcrun notarytool submit "$zip" --keychain-profile "$PROFILE" --wait
done

echo "notarization complete"
