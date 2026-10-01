<#
.SYNOPSIS
Builds release archives for YTGrab.

.DESCRIPTION
Runs the tests, then builds a Windows x64 zip containing ytgrab.exe, a start script, the
user guide, and tools\yt-dlp.exe. yt-dlp is bundled only when tools\yt-dlp.exe matches
the official checksum in tools\SHA2-256SUMS (download both from
https://github.com/yt-dlp/yt-dlp/releases). Each archive carries manifest.json (versions
and hashes) and SHA256SUMS. With -AllPlatforms it also builds Linux and macOS archives
that contain only the program.

.EXAMPLE
powershell -ExecutionPolicy Bypass -File scripts\package.ps1 -Version 1.0.0
#>
param(
    [string]$Version = '',
    [string]$OutDir = 'dist',
    [switch]$AllPlatforms,
    [switch]$SkipTests
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

function Invoke-Checked([string]$File, [string[]]$Arguments) {
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE" }
}

function Get-Sha256([string]$Path) {
    (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
}

function Write-Utf8NoBom([string]$Path, [string]$Text) {
    [System.IO.File]::WriteAllText($Path, $Text, (New-Object System.Text.UTF8Encoding $false))
}

$commit = (git rev-parse --short HEAD).Trim()
$dirty = (git status --porcelain) -ne $null
if (-not $Version) {
    $Version = (git describe --tags --always --dirty).Trim()
}
Write-Host "Packaging YTGrab $Version (commit $commit$(if ($dirty) { ', uncommitted changes' }))"

if (-not $SkipTests) {
    Invoke-Checked 'go' @('test', './...')
}

$dist = Join-Path $root $OutDir
New-Item -ItemType Directory -Force -Path $dist | Out-Null
$ldflags = "-s -w -X main.version=$Version"
$built = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')

function New-Package([string]$Os, [string]$Arch, [bool]$BundleTools) {
    $name = "ytgrab-$Version-$Os-$Arch"
    $stage = Join-Path $dist $name
    if (Test-Path $stage) { Remove-Item -Recurse -Force $stage }
    New-Item -ItemType Directory -Path $stage | Out-Null

    $exe = if ($Os -eq 'windows') { 'ytgrab.exe' } else { 'ytgrab' }
    $env:GOOS = $Os; $env:GOARCH = $Arch; $env:CGO_ENABLED = '0'
    try {
        Invoke-Checked 'go' @('build', '-trimpath', '-ldflags', $ldflags, '-o', (Join-Path $stage $exe), './cmd/ytgrab')
    } finally {
        Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    }
    Copy-Item (Join-Path $root 'docs\USER_GUIDE.md') (Join-Path $stage 'README.md')
    Copy-Item (Join-Path $root 'LICENSE') (Join-Path $stage 'LICENSE.txt')

    $tools = @()
    if ($Os -eq 'windows') {
        Copy-Item (Join-Path $root 'packaging\Start YTGrab.cmd') $stage
    }
    if ($BundleTools) {
        $ytdlp = Join-Path $root 'tools\yt-dlp.exe'
        $sums = Join-Path $root 'tools\SHA2-256SUMS'
        if ((Test-Path $ytdlp) -and (Test-Path $sums)) {
            $expected = (Get-Content $sums | Where-Object { $_ -match '\s\*?yt-dlp\.exe$' } | ForEach-Object { ($_ -split '\s+')[0].ToLowerInvariant() }) | Select-Object -First 1
            $actual = Get-Sha256 $ytdlp
            if (-not $expected -or $expected -ne $actual) {
                throw "tools\yt-dlp.exe ($actual) does not match the official checksum ($expected). Download both files again."
            }
            New-Item -ItemType Directory -Path (Join-Path $stage 'tools') | Out-Null
            Copy-Item $ytdlp (Join-Path $stage 'tools\yt-dlp.exe')
            $ytdlpVersion = (& $ytdlp --version).Trim()
            $tools += [ordered]@{
                name     = 'yt-dlp'
                version  = $ytdlpVersion
                file     = 'tools/yt-dlp.exe'
                sha256   = $actual
                source   = "https://github.com/yt-dlp/yt-dlp/releases/tag/$ytdlpVersion"
                verified = 'Matched the official SHA2-256SUMS at build time.'
            }
        } else {
            Write-Warning 'tools\yt-dlp.exe or tools\SHA2-256SUMS is missing; the Windows package will not include yt-dlp.'
        }
    }

    $manifest = [ordered]@{
        name     = 'ytgrab'
        version  = $Version
        commit   = $commit
        built    = $built
        platform = "$Os/$Arch"
        tools    = $tools
        notes    = 'ffmpeg, ffprobe, and a JavaScript runtime (Deno or Node) are installed separately; see README.md.'
    }
    Write-Utf8NoBom (Join-Path $stage 'manifest.json') ($manifest | ConvertTo-Json -Depth 5)

    # SHA256SUMS covers every file in the package, in sha256sum format.
    $lines = Get-ChildItem -Recurse -File $stage | Sort-Object FullName | ForEach-Object {
        $relative = $_.FullName.Substring($stage.Length + 1).Replace('\', '/')
        "$(Get-Sha256 $_.FullName)  $relative"
    }
    Write-Utf8NoBom (Join-Path $stage 'SHA256SUMS') (($lines -join "`n") + "`n")

    if ($Os -eq 'windows') {
        $archive = Join-Path $dist "$name.zip"
        if (Test-Path $archive) { Remove-Item -Force $archive }
        # Windows PowerShell's Compress-Archive writes backslash entry names; bsdtar
        # (tar.exe, built into Windows 10+) writes a standard zip.
        Invoke-Checked 'tar' @('-a', '-cf', $archive, '-C', $stage, '*')
    } else {
        $archive = Join-Path $dist "$name.tar.gz"
        if (Test-Path $archive) { Remove-Item -Force $archive }
        Invoke-Checked 'tar' @('-czf', $archive, '-C', $stage, '.')
    }
    Write-Utf8NoBom "$archive.sha256" "$(Get-Sha256 $archive)  $(Split-Path -Leaf $archive)`n"
    Remove-Item -Recurse -Force $stage
    Write-Host "Built $archive"
}

New-Package 'windows' 'amd64' $true
if ($AllPlatforms) {
    New-Package 'linux' 'amd64' $false
    New-Package 'linux' 'arm64' $false
    New-Package 'darwin' 'arm64' $false
    New-Package 'darwin' 'amd64' $false
}
