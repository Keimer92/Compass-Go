@echo off
setlocal
title Instalando Compass Go...
echo.
echo =============================================
echo   Compass Go - Compilar + Instalar
echo =============================================
echo.

REM ---------------------------------------------------------------
REM  Flags:  install.bat --force   rehace tambien los menus y la cascada
REM          install.bat --vibe    vuelve a configurar Vibe Coding
REM ---------------------------------------------------------------
set "FORCE=0"
set "WITH_VIBE=0"
:parse
if "%~1"=="" goto parsed
if /i "%~1"=="--force" set "FORCE=1"
if /i "%~1"=="-f"      set "FORCE=1"
if /i "%~1"=="--vibe"  set "WITH_VIBE=1"
shift
goto parse
:parsed

set "BINDIR=%LOCALAPPDATA%\Programs\Keimer"
set "LEGACYBIN=%USERPROFILE%\.cargo\bin"
set "BUILDDIR=bin"

where go >nul 2>nul
if errorlevel 1 (
    echo [ERROR] Go no esta instalado o no esta en el PATH.
    echo.
    echo   1. Ve a https://go.dev/dl/
    echo   2. Instala Go y abre una consola nueva
    echo   3. Vuelve a ejecutar este instalador
    echo.
    pause
    exit /b 1
)

if not exist "go.mod" (
    echo [ERROR] No se encuentra go.mod.
    echo         Ejecuta install.bat desde la raiz del proyecto Compass Go.
    echo.
    pause
    exit /b 1
)

REM Windows no deja sobrescribir un ejecutable en uso: se cierra primero.
REM `%SystemRoot%\System32\find.exe` explicito: si el PATH trae el find de
REM Git/MSYS antes que el de Windows, `find /i` falla y la deteccion miente.
tasklist /fi "imagename eq compass-go.exe" 2>nul | "%SystemRoot%\System32\find.exe" /i "compass-go.exe" >nul
if not errorlevel 1 (
    echo [INFO] Cerrando compass-go.exe en ejecucion...
    taskkill /f /im compass-go.exe >nul 2>nul
    if errorlevel 1 (
        echo [WARN] No se pudo cerrar compass-go.exe. Cierralo a mano y reintenta.
    ) else (
        echo [OK] compass-go.exe cerrado.
    )
    REM `timeout` protesta si el stdin esta redirigido; la pausa es opcional.
    timeout /t 1 /nobreak >nul 2>nul
)
echo.

REM ---------------------------------------------------------------
REM  1. Compilar (a bin\, fuera de la instalacion)
REM ---------------------------------------------------------------
echo [INFO] Compilando en %BUILDDIR%...
if not exist "%BUILDDIR%" mkdir "%BUILDDIR%"
where go-winres >nul 2>nul
if not errorlevel 1 (
    echo [INFO] Generando recursos de Windows con go-winres...
    go-winres make --in go-winres.json --out cmd\compass\rsrc --arch amd64,386 >nul
    if errorlevel 1 echo [WARN] go-winres fallo; se compila sin recursos.
) else (
    echo [AVISO] go-winres no encontrado: se compila sin icono ni version.
)
go build -o "%BUILDDIR%\compass-go.exe" ./cmd/compass
if errorlevel 1 (
    echo.
    echo =============================================
    echo   [ERROR] La compilacion fallo.
    echo =============================================
    echo La instalacion actual no se ha tocado.
    echo.
    pause
    exit /b 1
)
echo [OK] %BUILDDIR%\compass-go.exe generado.
echo.

REM ---------------------------------------------------------------
REM  2. Copiar binario e iconos
REM ---------------------------------------------------------------
if not exist "%BINDIR%" mkdir "%BINDIR%"

echo [INFO] Instalando en %BINDIR%...
copy /Y "%BUILDDIR%\compass-go.exe" "%BINDIR%\compass-go.exe" >nul
if errorlevel 1 (
    echo [ERROR] No se pudo copiar %BUILDDIR%\compass-go.exe a %BINDIR%.
    pause
    exit /b 1
)
echo [OK] compass-go.exe instalado.

REM Los iconos se resuelven junto al ejecutable (icon.ico / Default.ico /
REM opencode.ico); sin ellos el menu contextual muestra el icono generico.
for %%I in (Icon.ico Default.ico opencode.ico) do (
    if exist "assets\icon\%%I" copy /Y "assets\icon\%%I" "%BINDIR%\%%I" >nul 2>nul
)
if not exist "%BINDIR%\Default.ico" if exist "%BINDIR%\Icon.ico" (
    copy /Y "%BINDIR%\Icon.ico" "%BINDIR%\Default.ico" >nul 2>nul
)
if not exist "%BINDIR%\opencode.ico" if exist "%BINDIR%\Default.ico" (
    copy /Y "%BINDIR%\Default.ico" "%BINDIR%\opencode.ico" >nul 2>nul
)
echo [OK] Iconos copiados.
echo.

REM ---------------------------------------------------------------
REM  2b. Core kits (motores canonicos, p.ej. whatsapp) junto al exe
REM ---------------------------------------------------------------
if exist "corekit" (
    echo [INFO] Copiando core kits a %BINDIR%...
    xcopy /E /I /Y "corekit" "%BINDIR%\corekit" >nul 2>nul
    echo [OK] Core kits copiados.
) else (
    echo [SKIP] No se encontro la carpeta corekit; se omite.
)
echo.

REM ---------------------------------------------------------------
REM  3. Migrar la instalacion antigua en .cargo\bin
REM ---------------------------------------------------------------
REM Compass vivia antes en .cargo\bin (carpeta de Rust). Se borra solo
REM compass-go.exe; los .ico se dejan porque otros tools (code-runner) los
REM comparten.
if exist "%LEGACYBIN%\compass-go.exe" (
    del /f /q "%LEGACYBIN%\compass-go.exe" >nul 2>nul
    if exist "%LEGACYBIN%\compass-go.exe" (
        echo [WARN] No se pudo borrar %LEGACYBIN%\compass-go.exe.
    ) else (
        echo [OK] Copia antigua de .cargo\bin eliminada.
    )
)
echo.

REM ---------------------------------------------------------------
REM  4. Asegurar Compass y los shims globales de npm en el PATH del usuario
REM ---------------------------------------------------------------
REM Se edita HKCU\Environment\Path conservando el tipo REG_EXPAND_SZ y sin
REM expandir variables existentes (%USERPROFILE%, etc.).
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$dirs=@('%BINDIR%'); $npm=Join-Path $env:APPDATA 'npm'; if(Test-Path -LiteralPath $npm -PathType Container){$dirs+=$npm}; $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment',$true); if($k){$v=[string]$k.GetValue('Path','',[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); foreach($d in $dirs){$paths=@($v -split ';' | ForEach-Object {$_.TrimEnd('\')}); if($paths -notcontains $d.TrimEnd('\')){if($v.TrimEnd(';') -eq ''){$v=$d}else{$v=$v.TrimEnd(';')+';'+$d}}}; $kind=try{$k.GetValueKind('Path')}catch{[Microsoft.Win32.RegistryValueKind]::ExpandString}; $k.SetValue('Path',$v,$kind); $k.Close()}"
if errorlevel 1 (
    echo [WARN] No se pudo actualizar el PATH del usuario.
    echo        Anade manualmente: %BINDIR% y %APPDATA%\npm
) else (
    echo [OK] PATH del usuario actualizado. Abre una terminal nueva para verlo.
)
echo.

REM ---------------------------------------------------------------
REM  5. Menu contextual de Compass
REM ---------------------------------------------------------------
echo [INFO] Configurando menu contextual de Compass...
"%BINDIR%\compass-go.exe" --task context-menu-install
if errorlevel 1 (
    echo [WARN] No se pudo instalar el menu contextual de Compass.
) else (
    echo [OK] Menu contextual de Compass instalado.
)
echo.

REM ---------------------------------------------------------------
REM  6. Vibe Coding (solo la primera vez o con --vibe)
REM ---------------------------------------------------------------
if "%FORCE%"=="1" set "WITH_VIBE=1"
reg query "HKCU\Software\Classes\Directory\ContextMenus\VibeCoding" >nul 2>nul
if errorlevel 1 set "WITH_VIBE=1"

if "%WITH_VIBE%"=="1" (
    echo [INFO] Configurando Vibe Coding...
    echo        ^(Se preguntara por las CLIs que falten^)
    "%BINDIR%\compass-go.exe" --task vibe-setup
    if errorlevel 1 (
        echo [WARN] No se pudo configurar Vibe Coding.
    ) else (
        echo [OK] Vibe Coding configurado.
    )
) else (
    echo [SKIP] Vibe Coding ya esta configurado. Usa install.bat --vibe para rehacerlo.
)
echo.

REM El icono de las entradas nuevas solo aparece al refrescar la cache.
ie4uinit.exe -show >nul 2>nul

echo =============================================
echo   [OK] Instalacion completada
echo =============================================
echo   Binario: %BINDIR%\compass-go.exe
echo.
echo Si el menu no aparece, reinicia el Explorador:
echo   taskkill /f /im explorer.exe ^& start explorer.exe
echo.
exit /b 0
