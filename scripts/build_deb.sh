#!/usr/bin/env bash
set -e

echo "============================================="
echo " 📦 Building Lynx Debian Package"
echo "============================================="

# Ensure debian packaging tools are installed
if ! command -v dpkg-buildpackage &> /dev/null; then
    echo "Error: dpkg-buildpackage is not installed."
    echo "Run: sudo apt-get install build-essential debhelper"
    exit 1
fi

chmod -R u=rwX,go=rX .
chmod -R 0755 debian
dpkg-buildpackage -us -uc -b

echo -e "\n✅ Done! Debian package has been exported to the parent directory (../lynx_*.deb)"
