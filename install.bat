@echo off
setlocal EnableDelayedExpansion
title Instalando Compass Go...
echo.
echo =============================================
echo   Compass Go - Compilar + Instalar + Empaquetar
echo =============================================
echo.

REM ---------------------------------------------------------------
REM  Flags:  install.bat --force   rehace tambien los menus y la cascada
REM          install.bat --vibe    vuelve a configurar Vibe Coding
REM          install.bat --no-dist omite la generacion del instalador
REM ---------------------------------------------------------------
set "FORCE=0"
set "WITH_VIBE=0"
set "MAKE_DIST=1"
:parse
if "%~1"=="" goto parsed
if /i "%~1"=="--force"   set "FORCE=1"
if /i "%~1"=="-f"        set "FORCE=1"
if /i "%~1"=="--vibe"    set "WITH_VIBE=1"
if /i "%~1"=="--no-dist" set "MAKE_DIST=0"
shift
goto parse
:parsed

set "BINDIR=%LOCALAPPDATA%\Programs\Keimer"
set "LEGACYBIN=%USERPROFILE%\.cargo\bin"
set "MAKENSIS=%ProgramFiles(x86)%\NSIS\makensis.exe"

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
REM  1. Compilar
REM ---------------------------------------------------------------
echo [INFO] Compilando...
call build.bat
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
echo.

REM ---------------------------------------------------------------
REM  2. Copiar binario e iconos
REM ---------------------------------------------------------------
if not exist "%BINDIR%" mkdir "%BINDIR%"

echo [INFO] Instalando en %BINDIR%...
copy /Y "compass-go.exe" "%BINDIR%\compass-go.exe" >nul
if errorlevel 1 (
    echo [ERROR] No se pudo copiar compass-go.exe a %BINDIR%.
    pause
    exit /b 1
)
echo [OK] compass-go.exe instalado.

REM Los iconos se resuelven junto al ejecutable (icon.ico / Default.ico /
REM opencode.ico); sin ellos el menu contextual muestra el icono generico.
for %%I in (Icon.ico Default.ico opencode.ico) do (
    if exist "assets\icon\%%I" (
        copy /Y "assets\icon\%%I" "%BINDIR%\%%I" >nul 2>nul
    )
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
REM  4. Asegurar %BINDIR% en el PATH del usuario
REM ---------------------------------------------------------------
REM Se edita HKCU\Environment\Path conservando el tipo REG_EXPAND_SZ y sin
REM expandir variables existentes (%USERPROFILE%, etc.).
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$d='%BINDIR%'; $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment',$true); if($k){$v=[string]$k.GetValue('Path','',[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); if(($v -split ';') -notcontains $d){if($v.TrimEnd(';') -eq ''){$nv=$d}else{$nv=$v.TrimEnd(';')+';'+$d}; $k.SetValue('Path',$nv,[Microsoft.Win32.RegistryValueKind]::ExpandString)}; $k.Close()}"
if errorlevel 1 (
    echo [WARN] No se pudo actualizar el PATH del usuario.
    echo        Anade manualmente: %BINDIR%
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

REM ---------------------------------------------------------------
REM  7. Generar el instalador autonomo en dist\
REM ---------------------------------------------------------------
if "%MAKE_DIST%"=="0" (
    echo [SKIP] Generacion de dist omitida (--no-dist^).
    goto done
)

echo [INFO] Preparando dist\payload...
if not exist "dist\payload" mkdir "dist\payload"
copy /Y "compass-go.exe" "dist\payload\compass-go.exe" >nul
for %%I in (Icon.ico Default.ico opencode.ico) do (
    if exist "assets\icon\%%I" (
        copy /Y "assets\icon\%%I" "dist\payload\%%I" >nul 2>nul
    )
)

if not exist "%MAKENSIS%" (
    echo [WARN] NSIS no esta instalado; se omite el instalador.
    echo        Instala NSIS desde https://nsis.sourceforge.io/ y repite,
    echo        o ejecuta dist\build_installer.bat.
    goto done
)

echo [INFO] Generando dist\CompassGo-Setup.exe...
pushd "dist"
"%MAKENSIS%" installer.nsi >nul
if errorlevel 1 (
    popd
    echo [WARN] No se pudo generar el instalador.
) else (
    popd
    echo [OK] dist\CompassGo-Setup.exe generado.
)
echo.

:done
REM El icono de las entradas nuevas solo aparece al refrescar la cache.
ie4uinit.exe -show >nul 2>nul

echo =============================================
echo   [OK] Instalacion completada
echo =============================================
echo   Binario:    %BINDIR%\compass-go.exe
if "%MAKE_DIST%"=="1" if exist "dist\CompassGo-Setup.exe" echo   Instalador: %CD%\dist\CompassGo-Setup.exe
echo.
echo Si el menu no aparece, reinicia el Explorador:
echo   taskkill /f /im explorer.exe ^& start explorer.exe
echo.
exit /b 0
