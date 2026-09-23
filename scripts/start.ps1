$ErrorActionPreference = "Stop"

$repositoryRoot = Split-Path -Parent $PSScriptRoot
Push-Location $repositoryRoot
try {
    docker compose up --build -d --wait
}
finally {
    Pop-Location
}
