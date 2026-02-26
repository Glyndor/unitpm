#!/usr/bin/env bash
set -e

# Get version from debian/changelog
if [ -f debian/changelog ]; then
    VERSION="v$(dpkg-parsechangelog -S Version | cut -d- -f1)"
else
    VERSION="unknown"
fi

# Get git commit
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Get build date
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS="-s -w -X 'github.com/Jaro-c/Lynx/internal/version.Version=$VERSION' -X 'github.com/Jaro-c/Lynx/internal/version.Commit=$COMMIT' -X 'github.com/Jaro-c/Lynx/internal/version.BuildDate=$BUILD_DATE'"

echo "============================================="
echo " 🦁 Building Lynx CLI for Linux (amd64)"
echo " Version: $VERSION"
echo " Commit:  $COMMIT"
echo " Date:    $BUILD_DATE"
echo "============================================="

GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$LDFLAGS" -o lynx_linux_amd64 ./cmd/lynx

echo -e "\n✅ Done! Binary saved as ./lynx_linux_amd64"
