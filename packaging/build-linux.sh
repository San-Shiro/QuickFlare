#!/usr/bin/env bash
# Build every Linux artefact for a release: raw binaries, tarballs, .deb, .rpm.
#
#     VERSION=0.3.1 bash packaging/build-linux.sh
#
# The packaging steps run in containers so this works from a Windows host with
# no Linux toolchain installed. Everything else is a plain Go cross-compile,
# which the CLI supports because it needs no cgo.
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="${VERSION:?set VERSION, e.g. VERSION=0.3.1}"
ARCHES="${ARCHES:-amd64 arm64}"
NFPM_IMAGE="${NFPM_IMAGE:-goreleaser/nfpm:latest}"
TAR_IMAGE="${TAR_IMAGE:-alpine:3}"

# Docker on Windows needs a Windows-style path for a bind mount; pwd -W gives
# one under Git Bash and falls back to pwd everywhere else.
HOSTPWD="$(pwd -W 2>/dev/null || pwd)"

rm -rf build/linux
mkdir -p build/linux

for arch in $ARCHES; do
    echo "==> Building linux/${arch}"
    mkdir -p "build/linux/${arch}"
    GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build \
        -trimpath -ldflags "-s -w" \
        -o "build/linux/${arch}/quickflare" ./cmd/quickflare-cli

    # A plain binary, for anyone who just wants the file. GitHub does not
    # preserve a mode on release assets, so this one always needs chmod +x -
    # which is why the tarball below exists and is the recommended download.
    cp "build/linux/${arch}/quickflare" "build/quickflare-${VERSION}-linux-${arch}"
done

echo "==> Building tarballs"
for arch in $ARCHES; do
    # Built inside a container, staged outside the bind mount.
    #
    # A binary cross-compiled on Windows carries no Unix execute bit, and tar
    # records what it is given: the first version of this script produced a
    # tarball that unpacked to "Permission denied". Staging on a real Linux
    # filesystem and setting the modes explicitly is the fix - the archive
    # then says 0755 regardless of what the build host thinks.
    MSYS_NO_PATHCONV=1 docker run --rm \
        -v "${HOSTPWD}:/work" -w /work \
        -e "ARCH=${arch}" -e "VERSION=${VERSION}" \
        "$TAR_IMAGE" sh -c '
            set -e
            stage=/stage
            rm -rf "$stage" && mkdir -p "$stage"
            cp "build/linux/${ARCH}/quickflare" "$stage/quickflare"
            cp LICENSE README.md CHANGELOG.md "$stage/"
            chmod 0755 "$stage/quickflare"
            chmod 0644 "$stage/LICENSE" "$stage"/*.md
            tar -czf "build/quickflare-${VERSION}-linux-${ARCH}.tar.gz" -C "$stage" .
        '
done

echo "==> Packaging .deb and .rpm"
for arch in $ARCHES; do
    # The config is rendered here rather than left to nfpm's environment
    # expansion: nfpm does not expand variables inside content globs, so a
    # templated source path silently matches nothing instead of failing.
    conf="build/nfpm-${arch}.yaml"
    sed -e "s#[\$]{ARCH}#${arch}#g" -e "s#[\$]{VERSION}#${VERSION}#g" \
        packaging/nfpm.yaml > "$conf"

    if grep -q '{ARCH}\|{VERSION}' "$conf"; then
        echo "error: placeholders left in $conf" >&2
        exit 1
    fi

    for pkg in deb rpm; do
        MSYS_NO_PATHCONV=1 docker run --rm \
            -v "${HOSTPWD}:/work" -w /work \
            "$NFPM_IMAGE" package \
            --config "/work/${conf}" \
            --target /work/build \
            --packager "$pkg"
    done
    rm -f "$conf"
done

echo "==> Verifying the tarballs unpack executable"
for arch in $ARCHES; do
    MSYS_NO_PATHCONV=1 docker run --rm \
        -v "${HOSTPWD}:/work" -w /work \
        -e "ARCH=${arch}" -e "VERSION=${VERSION}" \
        "$TAR_IMAGE" sh -c '
            set -e
            rm -rf /t && mkdir -p /t
            tar -xzf "build/quickflare-${VERSION}-linux-${ARCH}.tar.gz" -C /t
            test -x /t/quickflare || { echo "    ${ARCH}: NOT EXECUTABLE" >&2; exit 1; }
            echo "    ${ARCH}: ok"
        '
done

echo
echo "==> Linux artefacts"
ls -1 build/quickflare-*linux-* build/*.deb build/*.rpm 2>/dev/null | sed 's/^/    /'
