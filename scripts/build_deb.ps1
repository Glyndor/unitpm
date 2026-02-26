$ErrorActionPreference = "Stop"

Write-Host "============================================="
Write-Host " 📦 Building Lynx Debian Package via WSL"
Write-Host "============================================="

$winPath = (Get-Location).Path
# Use wslpath to gracefully convert windows C:\ path into /mnt/c/ path without hardcoding
$wslPath = wsl wslpath -u "'$winPath'"
# Remove trailing newlines from output
$wslPath = $wslPath.Trim()

bash -c "rm -rf /tmp/lynx-build; cp -r . /tmp/lynx-build; cd /tmp/lynx-build; chmod -R u=rwX,go=rX .; chmod -R 0755 debian; dpkg-buildpackage -us -uc -b; cp ../lynx_*.deb '$wslPath/'"

Write-Host "`n✅ Done! Debian package has been exported to the project root."
