# Local local package, no PATH changes or network. PowerShell 7+.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
if ($arch -eq 'x64') { $arch = 'amd64' }
if ($arch -notin @('amd64','arm64')) { throw "Unsupported architecture: $arch" }
$bin = Join-Path $root "dist/release/ob_windows_$arch.exe"
$dataset = Join-Path $root 'dist/release/catalog.json.gz'
if (!(Test-Path $bin)) { throw 'Build local packages first.' }
$command = 'query'
$rest = @()
if ($args.Count -gt 0) { $command = $args[0]; $rest = @($args | Select-Object -Skip 1) }
if ($command -in @('query','search','show','status')) { & $bin $command --dataset $dataset @rest } else { & $bin $command @rest }
exit $LASTEXITCODE
