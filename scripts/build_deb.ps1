$ErrorActionPreference = "Stop"

Write-Host "============================================="
Write-Host " 📦 Building Lynx Debian Package via WSL"
Write-Host "============================================="

wsl -- sh -c "rm -rf /tmp/lynx-build && cp -r . /tmp/lynx-build && cd /tmp/lynx-build && chmod -R u=rwX,go=rX . && chmod -R 0755 debian && dpkg-buildpackage -us -uc -b && cp ../lynx_*.deb /mnt/j/Lynx/"

Write-Host "`n✅ Done! Debian package has been exported to the project root (J:\Lynx\)"
