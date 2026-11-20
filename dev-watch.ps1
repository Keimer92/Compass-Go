# dev-watch.ps1 — recompila y reinicia Compass Go cuando cambia el codigo Go.
#
# Lo llama dev.bat despues de levantar Vite. Vite se encarga del HMR de la UI
# (Svelte/CSS/TS); este script cubre el backend: vigila los .go, go.mod y go.sum,
# y en cada cambio mata la instancia anterior, recompila y la relanza.
#
# Reiniciar el proceso implica que las ConPTY abiertas mueren (son hijas del
# proceso), a diferencia de una recarga de pagina, que las conserva. Es
# inevitable: el binario nuevo necesita un proceso nuevo.
#
# Uso:  dev.bat            (o)  powershell -ExecutionPolicy Bypass -File dev-watch.ps1

$ErrorActionPreference = 'Continue'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root

$exe = Join-Path $root 'build\dev\compass-go.exe'
$exeDir = Split-Path $exe
if (-not (Test-Path $exeDir)) { New-Item -ItemType Directory -Path $exeDir | Out-Null }

# Argumentos extra de dev.bat (p. ej. -p "C:\ruta\proyecto"); --dev va siempre.
$appArgs = @('--dev') + $args

# Carpetas que no contienen codigo de Compass (o que cambian sin parar).
$skipDirs = 'frontend|build|dist|corekit|node_modules|\.git|\.idea|\.vscode'

function Get-SourceHash {
    $files = @()
    $files += Get-ChildItem -Path $root -Recurse -File -Filter '*.go' -ErrorAction SilentlyContinue
    foreach ($name in @('go.mod', 'go.sum')) {
        $p = Join-Path $root $name
        if (Test-Path $p) { $files += Get-Item $p }
    }
    $files = $files | Where-Object { $_.FullName -notmatch "[\\/]($skipDirs)[\\/]" }
    ($files | Sort-Object FullName | ForEach-Object {
        "$($_.FullName)|$($_.LastWriteTimeUtc.Ticks)|$($_.Length)"
    }) -join "`n"
}

function Build-App {
    Write-Host '[dev] Compilando...' -ForegroundColor Cyan
    # Sin `2>&1`: con ErrorActionPreference Stop los errores de un comando
    # nativo redirigidos a la salida se convierten en excepciones en PS 5.1.
    # Asi el compilador escribe sus errores directamente en la consola.
    & go build -o $exe ./cmd/compass-web
    if ($LASTEXITCODE -ne 0) {
        Write-Host '[dev] La compilacion fallo. Se espera el siguiente cambio.' -ForegroundColor Yellow
        return $false
    }
    return $true
}

# Espera a que el codigo cambie respecto a $hash y devuelve el hash nuevo.
function Wait-ForChange([string]$hash) {
    while ($true) {
        Start-Sleep -Milliseconds 600
        $now = Get-SourceHash
        if ($now -ne $hash) { return $now }
    }
}

$hash = Get-SourceHash
# Si el primer build falla se espera al siguiente cambio en vez de abortar:
# es el caso normal al empezar a escribir codigo que aun no compila.
while (-not (Build-App)) {
    $hash = Wait-ForChange $hash
}

Write-Host ''
Write-Host '[dev] Vigilando cambios en el codigo Go (Ctrl+C para salir).' -ForegroundColor Green
Write-Host '      Los cambios de UI siguen siendo HMR de Vite.'
Write-Host ''

$child = $null
try {
    while ($true) {
        $child = Start-Process -FilePath $exe -ArgumentList $appArgs -WorkingDirectory $root -PassThru

        # Se espera a que cambie el codigo o a que el usuario cierre Compass.
        $restart = $false
        while (-not $child.HasExited) {
            Start-Sleep -Milliseconds 600
            $child.Refresh()
            if ($child.HasExited) { break }
            $now = Get-SourceHash
            if ($now -ne $hash) { $hash = $now; $restart = $true; break }
        }

        if (-not $restart) {
            Write-Host '[dev] Compass Go se cerro. Fin del modo desarrollo.'
            break
        }

        # Matar antes de compilar: Windows no deja sobrescribir un .exe en uso.
        if (-not $child.HasExited) {
            Stop-Process -Id $child.Id -Force -ErrorAction SilentlyContinue
            $child.WaitForExit(3000) | Out-Null
        }
        $child = $null
        Start-Sleep -Milliseconds 300

        # Un cambio puede dejar el codigo a medias: se reintenta con el
        # siguiente guardado en lugar de terminar la sesion de desarrollo.
        while (-not (Build-App)) {
            $hash = Wait-ForChange $hash
        }
    }
}
finally {
    if ($child -and -not $child.HasExited) {
        Stop-Process -Id $child.Id -Force -ErrorAction SilentlyContinue
    }
}
