# dev-run.ps1 - supervisor del backend de Compass Go en modo desarrollo.
#
# Lo llama dev.bat en primer plano. Vigila los fuentes de Go y reinicia el
# backend solo al guardar un cambio (hot restart); ademas escucha el teclado en
# esta misma consola:
#   R  recompila y reinicia ahora mismo
#   Q  cierra Compass Go y sale
#
# Go se compila a un binario, asi que no existe hot reload real como en
# Flutter: cada reinicio recompila PRIMERO (la cache de `go build` lo deja en
# ~1 s) y solo reinicia si la compilacion paso. Si hay un error de Go, el
# Compass que ya esta abierto sigue vivo y solo se muestran los errores; el
# siguiente guardado (o R) lo reintenta.
#
# El binario se compila a build\dev\compass-web.exe. Se compila a un nombre
# aparte y se renombra al reiniciar porque Windows no deja sobrescribir un .exe
# en ejecucion.

param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$AppArgs
)

$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

$devDir = Join-Path $PSScriptRoot 'build\dev'
$bin = Join-Path $devDir 'compass-web.exe'
$next = Join-Path $devDir 'compass-web.next.exe'

# --dev sirve la UI desde Vite; los demas argumentos vienen de dev.bat.
$appArgs = @('--dev') + $AppArgs

# Cuanto hay que esperar sin cambios antes de recompilar: guardar varios
# archivos seguidos produce un solo reinicio.
$quietMs = 600
# Cada cuanto se consulta el watcher (y se revisan las teclas).
$pollMs = 150

$script:proc = $null
$script:everRan = $false
$script:keys = $true
$script:watchers = @()

# Estado compartido con el evento del watcher (corre en otro runspace): un
# hashtable sincronizado evita carreras al marcar/leer el cambio pendiente.
# Con eventos no se pierde ningun guardado aunque ocurra durante un rebuild.
$script:watch = [hashtable]::Synchronized(@{
        Pending = $false
        Last    = [datetime]::MinValue
    })

function Write-Dev([string]$Message, [string]$Color = 'Cyan') {
    Write-Host "[dev] $Message" -ForegroundColor $Color
}

# El Compass de dev se identifica por --dev en su linea de comandos: la
# version instalada no lo lleva y nunca se toca.
function Get-DevApp {
    Get-CimInstance Win32_Process |
        Where-Object {
            $_.CommandLine -like '*--dev*' -and
            $_.Name -in @('compass-web.exe', 'compass-go.exe')
        }
}

function Stop-Backend {
    # Primero un cierre ordenado: Wails termina sus ConPTY y no quedan CLIs
    # huerfanas. Si la ventana no responde en 5 s, se mata el arbol.
    foreach ($p in @(Get-DevApp)) {
        try {
            $gp = Get-Process -Id $p.ProcessId -ErrorAction Stop
            $gp.Refresh()
            [void]$gp.CloseMainWindow()
        } catch { }
    }
    $deadline = (Get-Date).AddSeconds(5)
    while ((Get-Date) -lt $deadline) {
        if (@(Get-DevApp).Count -eq 0) { break }
        Start-Sleep -Milliseconds 100
    }
    foreach ($p in @(Get-DevApp)) {
        & taskkill /PID $p.ProcessId /T /F 2>$null | Out-Null
    }
    if ($script:proc) {
        try {
            if (-not $script:proc.HasExited) { [void]$script:proc.WaitForExit(3000) }
        } catch { }
    }
}

function Build-Backend {
    if (-not (Test-Path -LiteralPath $devDir)) {
        New-Item -ItemType Directory -Path $devDir -Force | Out-Null
    }
    if (Test-Path -LiteralPath $next) { Remove-Item -LiteralPath $next -Force }
    & go build -o $next ./cmd/compass-web
    return ($LASTEXITCODE -eq 0)
}

function Quote-Arg([string]$Arg) {
    if ($Arg -match '[\s"]') { return '"' + ($Arg -replace '"', '\"') + '"' }
    return $Arg
}

function Start-Backend {
    if (Test-Path -LiteralPath $bin) { Remove-Item -LiteralPath $bin -Force }
    Move-Item -LiteralPath $next -Destination $bin -Force
    $argLine = (($appArgs | ForEach-Object { Quote-Arg $_ }) -join ' ')
    $script:proc = Start-Process -FilePath $bin -ArgumentList $argLine -PassThru
    $script:everRan = $true
}

function Read-KeyIfAny {
    if (-not $script:keys) { return '' }
    try {
        if (-not [Console]::KeyAvailable) { return '' }
        $name = [Console]::ReadKey($true).Key.ToString()
        if ($name -eq 'R') { return 'R' }
        if ($name -eq 'Q') { return 'Q' }
        return ''
    } catch {
        # Entrada redirigida: sin teclas, pero el backend sigue funcionando.
        $script:keys = $false
        Write-Dev 'No se puede leer el teclado en esta consola; reinicia con dev.bat.' 'Yellow'
        return ''
    }
}

function Rebuild-And-Restart([string]$Reason) {
    Write-Host ''
    Write-Dev "$Reason. Recompilando el backend..."
    if (-not (Build-Backend)) {
        Write-Dev 'La compilacion fallo. El Compass abierto sigue en pie; corrige y guarda otra vez (o pulsa R).' 'Yellow'
        return
    }
    Write-Dev 'Reiniciando Compass Go...'
    Stop-Backend
    Start-Backend
    Write-Host '       R = reiniciar ahora   |   Q = salir' -ForegroundColor DarkGray
}

# Vigila solo fuentes de Go y el modulo: el frontend ya lo recarga Vite.
# Se usan eventos (no WaitForChanged) porque el evento se encola aunque en ese
# momento se este compilando o reiniciando: un guardado durante el rebuild no
# se pierde y se aplica en la vuelta siguiente.
function Start-GoWatcher {
    # Dos watchers: `*.go` en todo el arbol (el filtro lo aplica Windows, asi
    # que node_modules no llena el buffer) y `go.*` solo en la raiz para
    # go.mod/go.sum.
    $goWatcher = New-Object IO.FileSystemWatcher $PSScriptRoot, '*.go'
    $goWatcher.IncludeSubdirectories = $true
    $goWatcher.NotifyFilter = [IO.NotifyFilters]'LastWrite, FileName, Size'
    $goWatcher.EnableRaisingEvents = $true

    $modWatcher = New-Object IO.FileSystemWatcher $PSScriptRoot, 'go.*'
    $modWatcher.IncludeSubdirectories = $false
    $modWatcher.NotifyFilter = [IO.NotifyFilters]'LastWrite, FileName, Size'
    $modWatcher.EnableRaisingEvents = $true

    $action = {
        $path = $Event.SourceEventArgs.FullPath
        if ($path -notmatch '\.(go|mod|sum)$') { return }
        if ($path -match '[\\/](build|dist|node_modules|\.git)[\\/]') { return }
        $Event.MessageData.Pending = $true
        $Event.MessageData.Last = [datetime]::UtcNow
    }
    # Cambios in-place (Changed) y guardados atomicos de los editores
    # (Created/Renamed): escuchar solo Changed perderia los ultimos.
    foreach ($w in @($goWatcher, $modWatcher)) {
        foreach ($evt in @('Changed', 'Created', 'Renamed')) {
            $null = Register-ObjectEvent -InputObject $w -EventName $evt -MessageData $script:watch -Action $action
        }
    }
    return @($goWatcher, $modWatcher)
}

# True cuando hubo un cambio y ya paso la ventana de silencio: guardar varios
# archivos seguidos produce un solo reinicio.
function Test-GoChange {
    if (-not $script:watch.Pending) { return $false }
    if (([datetime]::UtcNow - $script:watch.Last).TotalMilliseconds -lt $quietMs) { return $false }
    $script:watch.Pending = $false
    return $true
}

$script:watchers = @(Start-GoWatcher)

Write-Host ''
Write-Dev 'Compilando el backend...'
if (Build-Backend) {
    Start-Backend
    Write-Dev 'Compass Go arrancado.'
    Write-Host '       Cambios en Go: recompila y reinicia solo' -ForegroundColor DarkGray
    Write-Host '       R = reiniciar ahora   |   Q = salir' -ForegroundColor DarkGray
} else {
    Write-Dev 'La compilacion fallo. Corrige el error: se reintenta al guardar (o pulsa R).' 'Yellow'
}

try {
    while ($true) {
        # Un guardado en Go reinicia solo, este el backend vivo o caido por un
        # build fallido.
        if (Test-GoChange) {
            Rebuild-And-Restart 'Cambio en Go detectado'
            continue
        }

        # Con backend vivo se vigilan las teclas.
        if ($script:proc -and -not $script:proc.HasExited) {
            $key = Read-KeyIfAny
            if ($key -eq 'R') { Rebuild-And-Restart 'Reinicio manual'; continue }
            if ($key -eq 'Q') {
                Write-Host ''
                Write-Dev 'Cerrando Compass Go...'
                break
            }
            Start-Sleep -Milliseconds $pollMs
            continue
        }

        # Sin backend: o lo cerro el usuario, o el primer build fallo.
        if ($script:everRan) {
            Write-Dev 'Compass Go se cerro.'
            break
        }
        $key = Read-KeyIfAny
        if ($key -eq 'R') { Rebuild-And-Restart 'Reinicio manual'; continue }
        if ($key -eq 'Q') { break }
        Start-Sleep -Milliseconds $pollMs
    }
} finally {
    Stop-Backend
    foreach ($w in $script:watchers) {
        $w.EnableRaisingEvents = $false
        try {
            Get-EventSubscriber |
                Where-Object { $_.SourceObject -eq $w } |
                Unregister-Event -Force -ErrorAction SilentlyContinue
        } catch { }
        $w.Dispose()
    }
}
