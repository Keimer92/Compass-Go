@echo off
REM Construye compass-go.exe con recursos embebidos y empaqueta el instalador
REM NSIS autónomo (CompassGo-Setup.exe) listo para distribuir.
setlocal EnableDelayedExpansion
set "ROOT=%~dp0.."

REM 1. Compilar (icono + version + manifest via go-winres).
pushd "%ROOT%"
call build.bat
if errorlevel 1 (
    echo [ERROR] La compilacion fallo.
    popd
    pause
    exit /b 1
)
popd

REM 2. Reunir los archivos que va a llevar el instalador.
if not exist "%ROOT%\dist\payload" mkdir "%ROOT%\dist\payload"
copy /Y "%ROOT%\compass-go.exe" "%ROOT%\dist\payload\compass-go.exe" >nul
if exist "%ROOT%\assets\icon\Icon.ico"     copy /Y "%ROOT%\assets\icon\Icon.ico"     "%ROOT%\dist\payload\Icon.ico"     >nul
if exist "%ROOT%\assets\icon\Default.ico"  copy /Y "%ROOT%\assets\icon\Default.ico"  "%ROOT%\dist\payload\Default.ico"  >nul
if exist "%ROOT%\assets\icon\opencode.ico" copy /Y "%ROOT%\assets\icon\opencode.ico" "%ROOT%\dist\payload\opencode.ico" >nul

REM 3. Generar el instalador con NSIS.
if not exist "%ProgramFiles(x86)%\NSIS\makensis.exe" (
    echo [ERROR] NSIS no esta instalado (makensis.exe no encontrado).
    echo         Descargalo de https://nsis.sourceforge.io/
    pause
    exit /b 1
)
echo [INFO] Generando el instalador...
pushd "%ROOT%\dist"
"%ProgramFiles(x86)%\NSIS\makensis.exe" "installer.nsi"
if errorlevel 1 (
    popd
    echo [ERROR] No se pudo generar el instalador.
    pause
    exit /b 1
)
popd

echo.
echo =============================================
echo   [OK] dist\CompassGo-Setup.exe generado
echo =============================================
echo.
echo Para distribuir: entrega CompassGo-Setup.exe. Al ejecutarlo instala
echo Compass Go en %%LOCALAPPDATA%%\Programs\Compass Go, lo anade al PATH y
echo registra el menu contextual. La desinstalacion queda en "Agregar o
echo quitar programas".
echo.
exit /b 0
