$ErrorActionPreference = "Stop"

# Get version from debian/changelog
$changelogLine = Get-Content .\debian\changelog -TotalCount 1
if ($changelogLine -match 'lynx \((.+?)-\d+\)') {
    $VERSION = "v" + $Matches[1]
} else {
    $VERSION = "unknown"
}

# Get git commit
try {
    $COMMIT = git rev-parse --short HEAD 2>$null
    if (-not $COMMIT) { $COMMIT = "unknown" }
} catch {
    $COMMIT = "unknown"
}

# Get build date
$BUILD_DATE = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

$LDFLAGS = "-s -w -X 'github.com/Jaro-c/Lynx/internal/version.Version=$VERSION' -X 'github.com/Jaro-c/Lynx/internal/version.Commit=$COMMIT' -X 'github.com/Jaro-c/Lynx/internal/version.BuildDate=$BUILD_DATE'"

Write-Host "============================================="
Write-Host " 🦁 Building Lynx CLI for Linux (amd64)"
Write-Host " Version: $VERSION"
Write-Host " Commit:  $COMMIT"
Write-Host " Date:    $BUILD_DATE"
Write-Host "============================================="

$env:GOOS = "linux"
$env:GOARCH = "amd64"

go build -trimpath -ldflags=$LDFLAGS -o lynx_linux_amd64 ./cmd/lynx

Write-Host "`n✅ Done! Binary saved as .\lynx_linux_amd64"
