param(
  [string]$ReleaseVersion = $(if ($env:AGENT_TELEMETRY_RELEASE_VERSION) { $env:AGENT_TELEMETRY_RELEASE_VERSION } else { "latest" }),
  [string]$GitHubRepo = $(if ($env:AGENT_TELEMETRY_GITHUB_REPO) { $env:AGENT_TELEMETRY_GITHUB_REPO } else { "GuanceCloud/agent-telemetry" }),
  [Parameter(ValueFromRemainingArguments = $true)]
  [string[]]$InstallArgs
)

$ErrorActionPreference = "Stop"

function Get-ArchiveName {
  param(
    [string]$ResolvedVersion,
    [string]$Platform
  )

  $normalized = $ResolvedVersion.TrimStart("v")
  if ($normalized -eq "latest") {
    return "agent-telemetry-$Platform.zip"
  }
  return "agent-telemetry-v$normalized-$Platform.zip"
}

function Get-BaseUrl {
  param(
    [string]$Repo,
    [string]$ResolvedVersion
  )

  $normalized = $ResolvedVersion.TrimStart("v")
  if ($normalized -eq "latest") {
    return "https://github.com/$Repo/releases/latest/download"
  }
  return "https://github.com/$Repo/releases/download/v$normalized"
}

function Get-ExpectedChecksum {
  param(
    [string]$ChecksumFile,
    [string]$AssetName
  )

  foreach ($line in Get-Content -Path $ChecksumFile) {
    if ($line -match '^([0-9a-fA-F]{64})\s+\*?(.+)$' -and $Matches[2] -eq $AssetName) {
      return $Matches[1].ToLowerInvariant()
    }
  }
  throw "Missing checksum entry for $AssetName"
}

$platformWindows = [System.Runtime.InteropServices.OSPlatform]::Windows
if (-not [System.Runtime.InteropServices.RuntimeInformation]::IsOSPlatform($platformWindows)) {
  throw "install-release.ps1 is only supported on Windows. Use install-release.sh on Linux or macOS."
}

$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
switch ($arch) {
  "x64" { $platform = "windows-amd64" }
  "arm64" { $platform = "windows-arm64" }
  default { throw "Unsupported architecture: $arch" }
}

$baseUrl = Get-BaseUrl -Repo $GitHubRepo -ResolvedVersion $ReleaseVersion
$archiveName = Get-ArchiveName -ResolvedVersion $ReleaseVersion -Platform $platform
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString("N"))
$null = New-Item -ItemType Directory -Path $tempRoot -Force

try {
  $checksumPath = Join-Path $tempRoot "SHA256SUMS"
  $archivePath = Join-Path $tempRoot $archiveName
  Invoke-WebRequest -Uri "$baseUrl/SHA256SUMS" -OutFile $checksumPath
  Invoke-WebRequest -Uri "$baseUrl/$archiveName" -OutFile $archivePath

  $expected = Get-ExpectedChecksum -ChecksumFile $checksumPath -AssetName $archiveName
  $actual = (Get-FileHash -Algorithm SHA256 -Path $archivePath).Hash.ToLowerInvariant()
  if ($expected -ne $actual) {
    throw "Checksum mismatch for $archiveName"
  }

  $extractDir = Join-Path $tempRoot "extract"
  Expand-Archive -Path $archivePath -DestinationPath $extractDir -Force
  $installer = Join-Path $extractDir "scripts\install.ps1"
  if (-not (Test-Path -Path $installer)) {
    throw "Release archive is missing scripts/install.ps1"
  }

  & $installer @InstallArgs
  exit $LASTEXITCODE
} finally {
  if (Test-Path -Path $tempRoot) {
    Remove-Item -Path $tempRoot -Recurse -Force
  }
}
