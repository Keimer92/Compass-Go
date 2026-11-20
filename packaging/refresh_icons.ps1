# Refresca la cache de iconos de Windows tras instalar/desinstalar Compass Go.
#
# Windows guarda una copia del icono de cada .exe/.lnk en su cache. Al cambiar
# assets\icon\CompassGo.ico y reinstalar, el acceso directo del escritorio, del
# menu Inicio y sobre todo el anclado a la barra de tareas pueden seguir
# mostrando el icono viejo durante dias. Este script lo fuerza.
#
# Dos modos:
#   - Suave (por defecto): SHChangeNotify + ie4uinit -show. No cierra Explorer.
#   - Completo (-Force, o cuando el .ico cambia respecto al ultimo stamp):
#     reinicia Explorer y borra iconcache_*.db / thumbcache_*.db.
#
# Uso:
#   powershell -File packaging\refresh_icons.ps1
#   powershell -File packaging\refresh_icons.ps1 -IconPath "%BINDIR%\CompassGo.ico" -StampPath "%LOCALAPPDATA%\compass\icon-stamp.txt"

param(
    [string]$IconPath = "",
    [string]$StampPath = "",
    [switch]$Force
)

$ErrorActionPreference = 'SilentlyContinue'

function Invoke-SoftRefresh {
    # SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, 0, 0): pide al shell que
    # relea iconos y asociaciones sin reiniciar Explorer. Si Add-Type esta
    # bloqueado (modo restringido), se sigue con ie4uinit.
    try {
        Add-Type -Namespace CompassShell -Name Native -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("shell32.dll")]
public static extern void SHChangeNotify(int eventId, int flags, System.IntPtr item1, System.IntPtr item2);
'@
        [CompassShell.Native]::SHChangeNotify(0x08000000, 0x0000, [IntPtr]::Zero, [IntPtr]::Zero)
    } catch {
        # Sin SHChangeNotify; ie4uinit suele bastar.
    }
    & "$env:SystemRoot\System32\ie4uinit.exe" -show 2>$null | Out-Null
}

function Invoke-FullRefresh {
    # Explorer mantiene abiertos los .db de la cache, asi que hay que cerrarlo
    # para poder borrarlos. Se reabre al terminar (y si Windows lo reinicia
    # solo, Start-Process no duplica la instancia).
    $explorerWasRunning = [bool](Get-Process -Name explorer -ErrorAction SilentlyContinue)
    if ($explorerWasRunning) {
        Stop-Process -Name explorer -Force -ErrorAction SilentlyContinue
        Start-Sleep -Milliseconds 800
    }

    $explorerDir = Join-Path $env:LOCALAPPDATA 'Microsoft\Windows\Explorer'
    Get-ChildItem -LiteralPath $explorerDir -Filter 'iconcache_*.db' -Force -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue
    Get-ChildItem -LiteralPath $explorerDir -Filter 'thumbcache_*.db' -Force -ErrorAction SilentlyContinue |
        Remove-Item -Force -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $env:LOCALAPPDATA 'IconCache.db') -Force -ErrorAction SilentlyContinue

    if ($explorerWasRunning -and -not (Get-Process -Name explorer -ErrorAction SilentlyContinue)) {
        Start-Process explorer.exe
        Start-Sleep -Milliseconds 500
    }

    Invoke-SoftRefresh
}

$needsFull = $Force.IsPresent

if ($IconPath -and $StampPath -and (Test-Path -LiteralPath $IconPath)) {
    $hash = (Get-FileHash -LiteralPath $IconPath -Algorithm SHA256).Hash
    $old = ''
    if (Test-Path -LiteralPath $StampPath) {
        $old = (Get-Content -LiteralPath $StampPath -Raw).Trim()
    }
    if ($hash -ne $old) {
        $needsFull = $true
        $dir = Split-Path -Parent $StampPath
        if ($dir -and -not (Test-Path -LiteralPath $dir)) {
            New-Item -ItemType Directory -Path $dir -Force | Out-Null
        }
        Set-Content -LiteralPath $StampPath -Value $hash -NoNewline
    }
}

if ($needsFull) {
    Write-Host '[INFO] Icono nuevo detectado: reconstruyendo la cache de iconos...'
    Invoke-FullRefresh
    Write-Host '[OK] Cache de iconos reconstruida.'
} else {
    Invoke-SoftRefresh
    Write-Host '[OK] Cache de iconos refrescada.'
}

exit 0
