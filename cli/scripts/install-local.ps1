# Offline bootstrap; Windows PowerShell 5.1+ / PowerShell 7+.
param([string]$Package=$PSScriptRoot,[Parameter(Mandatory=$true)][string]$Dir)
$ErrorActionPreference='Stop'
$architecture=$env:PROCESSOR_ARCHITECTURE.ToLowerInvariant()
if ($env:PROCESSOR_ARCHITEW6432) { $architecture=$env:PROCESSOR_ARCHITEW6432.ToLowerInvariant() }
if ($architecture -eq 'x86_64') { $architecture='amd64' }
if ($architecture -notin @('amd64','arm64')) { throw "Unsupported architecture: $architecture" }
$Package=(Resolve-Path -LiteralPath $Package).Path
$name="ob_windows_$architecture.exe"
$binary=Join-Path $Package $name
if (!(Test-Path -LiteralPath $binary -PathType Leaf)) { throw "Missing binary: $name" }
if ((Get-Item -LiteralPath $binary).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Binary symlinks/reparse points are not accepted' }
$checksums=Join-Path $Package 'CLI-SHA256SUMS'
if ((Get-Item -LiteralPath $checksums).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw 'Checksums symlinks/reparse points are not accepted' }
$lines=@(Get-Content -LiteralPath $checksums | Where-Object { ($_ -split '\s+').Count -eq 2 -and ($_ -split '\s+')[1] -eq $name })
if ($lines.Count -ne 1) { throw 'Missing/duplicate binary checksum' }
$expected=($lines[0] -split '\s+')[0]
if ($expected -notmatch '^[a-f0-9]{64}$') { throw 'Invalid checksum' }
$actual=(Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw 'Binary checksum mismatch; nothing executed or installed' }
& $binary install --package $Package --dir $Dir
exit $LASTEXITCODE
