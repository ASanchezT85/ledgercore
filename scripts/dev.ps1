# LedgerCore — local dev stack helper for Windows (PowerShell equivalent of
# the Makefile compose targets: dev / dev-obs / down).
#
# Usage, from anywhere:
#   .\scripts\dev.ps1               # make dev      -> compose up -d --build
#   .\scripts\dev.ps1 -Obs          # make dev-obs  -> adds grafana/otel-lgtm
#   .\scripts\dev.ps1 -Web          # adds the console (profile web)
#   .\scripts\dev.ps1 -Obs -Web     # both optional profiles
#   .\scripts\dev.ps1 -Down         # make down     -> compose down
#   .\scripts\dev.ps1 -Down -Volumes# compose down -v (drops the Postgres data)

param(
    [switch]$Obs,     # also start the observability profile (grafana/otel-lgtm)
    [switch]$Web,     # also start the web console (apps/console)
    [switch]$Down,    # stop the stack instead of starting it
    [switch]$Volumes  # with -Down: also remove named volumes (postgres data)
)

$ErrorActionPreference = 'Stop'

# Resolve the compose file relative to this script so the location the user
# calls it from does not matter.
$repoRoot = Split-Path -Parent $PSScriptRoot
$composeFile = Join-Path $repoRoot 'infra\compose\docker-compose.yml'

# Base arguments; profiles must be passed before the up/down subcommand.
$composeArgs = @('compose', '-f', $composeFile)
if ($Obs) { $composeArgs += @('--profile', 'obs') }
if ($Web) { $composeArgs += @('--profile', 'web') }

if ($Down) {
    $composeArgs += 'down'
    if ($Volumes) { $composeArgs += '-v' }
} else {
    # -d: detached; --build: rebuild service images when sources changed.
    $composeArgs += @('up', '-d', '--build')
}

& docker @composeArgs
exit $LASTEXITCODE
