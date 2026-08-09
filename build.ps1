[CmdletBinding()]
param(
    [string]$BuildNumber = (Get-Date).ToUniversalTime().ToString("yyyyMMddHHmmss"),
    [ValidateSet("amd64", "arm64")]
    [string[]]$Architecture = @("amd64", "arm64"),
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"
$projectDirectory = $PSScriptRoot
$outputDirectory = Join-Path $projectDirectory "dist"
$mainSource = Get-Content -LiteralPath (Join-Path $projectDirectory "main.go") -Raw
if ($mainSource -notmatch 'version\s*=\s*"([^"]+)"') {
    throw "Could not read the version from main.go."
}
$Version = $Matches[1]
$safeVersion = $Version -replace '[^0-9A-Za-z._-]', '_'
$safeBuildNumber = $BuildNumber -replace '[^0-9A-Za-z._-]', '_'
$previousGOOS = $env:GOOS
$previousGOARCH = $env:GOARCH
$previousGOCACHE = $env:GOCACHE
$previousGOMODCACHE = $env:GOMODCACHE

Push-Location $projectDirectory
try {
    $env:GOCACHE = Join-Path $projectDirectory ".cache\go-build"
    $env:GOMODCACHE = Join-Path $projectDirectory ".cache\go-mod"
    if (-not $SkipTests) {
        & go test ./...
        if ($LASTEXITCODE -ne 0) { throw "Go tests failed." }
    }

    New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
    $env:GOOS = "windows"
    $linkerFlags = "-H=windowsgui -s -w -X main.buildNumber=$BuildNumber"
	$builtAssets = @{}

    foreach ($targetArchitecture in $Architecture) {
        $env:GOARCH = $targetArchitecture
        $outputPath = Join-Path $outputDirectory "zzz-wallpaper-companion-$safeVersion-build-$safeBuildNumber-windows-$targetArchitecture.exe"
        & go build -trimpath -ldflags $linkerFlags -o $outputPath .
        if ($LASTEXITCODE -ne 0) { throw "Go build failed for $targetArchitecture." }

        $artifact = Get-Item -LiteralPath $outputPath
        Write-Host "Built $($artifact.FullName)"
        Write-Host "Version $Version, build $BuildNumber, $targetArchitecture, $([math]::Round($artifact.Length / 1MB, 2)) MB"
		$builtAssets["windows-$targetArchitecture"] = @{
			name = $artifact.Name
			sha256 = (Get-FileHash -Algorithm SHA256 -LiteralPath $artifact.FullName).Hash.ToLowerInvariant()
		}
    }

	$manifest = @{
		version = $Version
		protocolMin = 2
		protocolMax = 2
		assets = $builtAssets
	} | ConvertTo-Json -Depth 4
	$manifestPath = Join-Path $outputDirectory "zzz-wallpaper-companion-update.json"
	[System.IO.File]::WriteAllText($manifestPath, $manifest + [Environment]::NewLine, (New-Object System.Text.UTF8Encoding($false)))
	Write-Host "Built $manifestPath"
} finally {
    $env:GOOS = $previousGOOS
    $env:GOARCH = $previousGOARCH
    $env:GOCACHE = $previousGOCACHE
    $env:GOMODCACHE = $previousGOMODCACHE
    Pop-Location
}
