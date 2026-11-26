#!/usr/bin/env bash
# Prints the path of bombadil at the version the checks in bombadil/spec.ts are written for. If neither
# bin/bombadil nor the bombadil on the PATH is that exact build, by its checksum, it downloads it into
# bin/ first and checks it against the checksum below. Bombadil breaks compatibility between minor
# versions, so a new version means checking the specification against it, then changing the version
# and every checksum here. GitHub shows each asset's checksum on the release page.
set -euo pipefail

version=0.8.0

case "$(uname -s)-$(uname -m)" in
Darwin-arm64)
  asset=bombadil-aarch64-darwin
  sha256=2ea26a68ac9b5296a8828ff138fb17a3343f7e8cf50647e9d1a81074505f995c
  ;;
Linux-x86_64)
  asset=bombadil-x86_64-linux
  sha256=d9d1d2832cb23c874744a4f20526d2064ce8571c31f8ecd01ddf8e1c44da4814
  ;;
Linux-aarch64)
  asset=bombadil-aarch64-linux
  sha256=8ac5e26b609296ec7017480220baf310617b9a799352c6fd5e723b2a30d246d3
  ;;
*)
  echo "bombadil $version has no build for $(uname -s) on $(uname -m)" >&2
  exit 1
  ;;
esac

checksum() {
  if command -v sha256sum >/dev/null; then
    sha256sum "$1" | cut -d ' ' -f 1
  else
    shasum -a 256 "$1" | cut -d ' ' -f 1
  fi
}

root="$(cd "$(dirname "$0")/.." && pwd)"

for candidate in "$root/bin/bombadil" "$(command -v bombadil || true)"; do
  if [ -n "$candidate" ] && [ -x "$candidate" ] && [ "$(checksum "$candidate")" = "$sha256" ]; then
    echo "$candidate"
    exit 0
  fi
done

mkdir -p "$root/bin"
download="$(mktemp "$root/bin/bombadil.XXXXXX")"
trap 'rm -f "$download"' EXIT
echo "Downloading bombadil $version" >&2
curl -fsSL --retry 3 -o "$download" "https://github.com/antithesishq/bombadil/releases/download/v$version/$asset"
if [ "$(checksum "$download")" != "$sha256" ]; then
  echo "The download of bombadil $version does not have the checksum it should, so it is not used" >&2
  exit 1
fi
chmod a+x "$download"
mv "$download" "$root/bin/bombadil"
echo "$root/bin/bombadil"
