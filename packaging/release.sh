#!/usr/bin/env bash
# Publishes the current tag to GitHub. Run after `gh auth login`.
#
# A tag with a suffix - v0.3.1-dev, v0.4.0-rc1 - is published as a
# pre-release. GitHub then keeps it off the "Latest release" banner and off
# the repository front page, which is the whole point of shipping one.
set -euo pipefail

OWNER="${OWNER:-San-Shiro}"
REPO="${REPO:-QuickFlare}"
TAG="$(git describe --tags --abbrev=0)"
VERSION="${TAG#v}"

# Windows Installer versions must be plain x.y.z - it has no concept of a
# pre-release suffix, and a non-numeric version is rejected outright. The
# package carries the numeric part; the tag carries the full story.
MSI_VERSION="${VERSION%%-*}"

PRERELEASE=""
if [ "$VERSION" != "$MSI_VERSION" ]; then
    PRERELEASE="--prerelease"
    echo "==> $TAG is a pre-release (package version $MSI_VERSION)"
fi

GH="$(find "$LOCALAPPDATA/Microsoft/WinGet" -name gh.exe 2>/dev/null | head -1)"
[ -n "$GH" ] || GH=gh

echo "==> Building Windows artefacts for $TAG"
go build -ldflags "-H windowsgui -s -w" -o "build/QuickFlare-$MSI_VERSION.exe" ./cmd/quickflare
cp -f "build/QuickFlare-$MSI_VERSION.exe" "build/QuickFlare.exe"

# -arch x64 is not optional. Without it WiX declares the package Intel (x86)
# while it ships an amd64 binary, and Windows then registers the product in
# the 32-bit registry view - the uninstall entry lands under WOW6432Node
# rather than where a per-user x64 install belongs.
PATH="$PATH:$HOME/.dotnet/tools" wix build packaging/quickflare.wxs -b . \
  -arch x64 -d "Version=$MSI_VERSION" -o "build/QuickFlare-$MSI_VERSION-x64.msi"
rm -f build/*.wixpdb

echo "==> Building Linux artefacts for $TAG"
VERSION="$MSI_VERSION" bash packaging/build-linux.sh

echo "==> Creating repository $OWNER/$REPO (skipped if it exists)"
"$GH" repo create "$OWNER/$REPO" \
  --public \
  --source=. \
  --remote=origin \
  --description "Publish localhost to the internet through Cloudflare Tunnel with cloudflared." \
  --push 2>/dev/null || {
    echo "    repository already exists; pushing instead"
    git remote get-url origin >/dev/null 2>&1 || \
      git remote add origin "https://github.com/$OWNER/$REPO.git"
    git push -u origin HEAD
  }

git push origin "$TAG"

echo "==> Publishing release $TAG"
"$GH" release create "$TAG" \
  --repo "$OWNER/$REPO" \
  --title "QuickFlare $TAG" \
  --notes-file CHANGELOG.md \
  $PRERELEASE \
  "build/QuickFlare-$MSI_VERSION-x64.msi#Windows installer (per-user, no admin)" \
  "build/QuickFlare-$MSI_VERSION.exe#Windows portable" \
  "build/quickflare_${MSI_VERSION}_amd64.deb#Debian/Ubuntu amd64" \
  "build/quickflare_${MSI_VERSION}_arm64.deb#Debian/Ubuntu arm64" \
  "build/quickflare-${MSI_VERSION}-1.x86_64.rpm#Fedora/RHEL x86_64" \
  "build/quickflare-${MSI_VERSION}-1.aarch64.rpm#Fedora/RHEL aarch64" \
  "build/quickflare-${MSI_VERSION}-linux-amd64.tar.gz#Linux amd64 tarball" \
  "build/quickflare-${MSI_VERSION}-linux-arm64.tar.gz#Linux arm64 tarball" \
  "build/quickflare-${MSI_VERSION}-linux-amd64#Linux amd64 binary (chmod +x)" \
  "build/quickflare-${MSI_VERSION}-linux-arm64#Linux arm64 binary (chmod +x)"

echo "==> Done: https://github.com/$OWNER/$REPO/releases/tag/$TAG"
