# dev-reset.ps1 — cierra la sesion de desarrollo anterior antes de abrir una
# nueva. Lo llama dev.bat al arrancar, ANTES de poner el titulo de la ventana,
# para no cerrarse a si mismo.
#
# Solo afecta a sesiones de desarrollo:
#   - el Compass de dev se identifica por el flag --dev en su linea de comandos
#     (la version instalada no lo lleva, asi que nunca se toca);
#   - la terminal vieja se identifica por el titulo que pone el propio dev.bat.
# Vite corre en otra ventana ("Compass Go - Vite") y se deja vivo a proposito:
# asi el HMR sigue sirviendo y no hay que volver a arrancarlo.

$ErrorActionPreference = 'SilentlyContinue'

# PID del cmd que invoco este script (padre del powershell). Se excluye para no
# matar la sesion nueva que se esta iniciando.
$parentPid = (Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId

# 1) Compass de dev + sus ConPTY hijas. /T limpia las terminales embebidas
#    huerfanas que el proceso pudiera dejar atras.
Get-CimInstance Win32_Process |
    Where-Object {
        $_.CommandLine -like '*--dev*' -and
        $_.Name -in @('compass-web.exe', 'compass-go.exe')
    } |
    ForEach-Object { & taskkill /PID $_.ProcessId /T /F 2>$null | Out-Null }

# 2) Terminal del dev.bat anterior. Sin /T: Vite es hija de ese cmd y debe
#    sobrevivir para mantener el HMR.
Get-Process cmd |
    Where-Object {
        $_.MainWindowTitle -eq 'Compass Go - desarrollo' -and
        $_.Id -ne $parentPid
    } |
    ForEach-Object { & taskkill /PID $_.Id /F 2>$null | Out-Null }
