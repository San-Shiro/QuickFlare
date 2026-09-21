#!/usr/bin/env bash
# Publishes the current tag to GitHub. Run after `gh auth login`.
set -euo pipefail

OWNER="${OWNER:-San-Shiro}"
REPO="${REPO:-QuickFlare}"
TAG="$(git describe --tags --abbrev=0)"
VERSION="${TAG#v}"

GH="$(find "$LOCALAPPDATA/Microsoft/WinGet" -name gh.exe 2>/dev/null | head -1)"
[ -n "$GH" ] || GH=gh

echo "==> Building release artefacts for $TAG"
go build -ldflags "-H windowsgui -s -w" -o "build/QuickFlare-$VERSION.exe" ./cmd/quickflare
cp -f "build/QuickFlare-$VERSION.exe" "build/QuickFlare.exe"
# -arch x64 is not optional. Without it WiX declares the package Intel (x86)
# while it ships an amd64 binary, and Windows then registers the product in
# the 32-bit registry view - the uninstall entry lands under WOW6432Node
# rather than where a per-user x64 install belongs.
PATH="$PATH:$HOME/.dotnet/tools" wix build packaging/quickflare.wxs -b . \
  -arch x64 -d "Version=$VERSION" -o "build/QuickFlare-$VERSION-x64.msi"
rm -f build/*.wixpdb

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
  "build/QuickFlare-$VERSION-x64.msi#QuickFlare-$VERSION-x64-Installer" \
  "build/QuickFlare-$VERSION.exe#QuickFlare-$VERSION-Portable"

echo "==> Done: https://github.com/$OWNER/$REPO/releases/tag/$TAG"
