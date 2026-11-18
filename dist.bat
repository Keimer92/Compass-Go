@echo off
setlocal
title Generando Compass Go en dist...
echo.
echo =============================================
echo   Compass Go - Generar instalador en dist\
echo =============================================
echo.

REM ---------------------------------------------------------------
REM  Flags:  dist.bat                 compila + genera el instalador
REM          dist.bat --no-installer  solo prepara dist\payload
REM          dist.bat --fast          compresion zlib (rapida, instalador mayor)
REM ---------------------------------------------------------------
set "MAKE_INSTALLER=1"
set "FAST=0"
:parse
if "%~1"=="" goto parsed
if /i "%~1"=="--no-installer" set "MAKE_INSTALLER=0"
if /i "%~1"=="--no-dist"      set "MAKE_INSTALLER=0"
if /i "%~1"=="--fast"         set "FAST=1"
shift
goto parse
:parsed

set "MAKENSIS=%ProgramFiles(x86)%\NSIS\makensis.exe"

where go >nul 2>nul
if errorlevel 1 (
    echo [ERROR] Go no esta instalado o no esta en el PATH.
    echo.
    echo   1. Ve a https://go.dev/dl/
    echo   2. Instala Go y abre una consola nueva
    echo   3. Vuelve a ejecutar dist.bat
    echo.
    pause
    exit /b 1
)

if not exist "go.mod" (
    echo [ERROR] No se encuentra go.mod.
    echo         Ejecuta dist.bat desde la raiz del proyecto Compass Go.
    echo.
    pause
    exit /b 1
)
echo.

REM ---------------------------------------------------------------
REM  0. Iconos .ico desde los .png de assets\icon (si faltan)
REM ---------------------------------------------------------------
if not exist "assets\icon\CompassGo.ico" if exist "assets\icon\Compass Go.png" (
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "packaging\make_icons.ps1" -Source "assets\icon\Compass Go.png" -Out "assets\icon\CompassGo.ico" >nul 2>nul
)
if not exist "assets\icon\VibeCoding.ico" if exist "assets\icon\Vibe Coding.png" (
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "packaging\make_icons.ps1" -Source "assets\icon\Vibe Coding.png" -Out "assets\icon\VibeCoding.ico" >nul 2>nul
)
if not exist "assets\icon\Git.ico" if exist "assets\icon\Git.png" (
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "packaging\make_icons.ps1" -Source "assets\icon\Git.png" -Out "assets\icon\Git.ico" >nul 2>nul
)

REM ---------------------------------------------------------------
REM  1. Compilar directo a dist\payload
REM ---------------------------------------------------------------
echo [INFO] Compilando en dist\payload...
if not exist "dist\payload" mkdir "dist\payload"
where go-winres >nul 2>nul
if not errorlevel 1 (
    echo [INFO] Generando recursos de Windows con go-winres...
    go-winres make --in go-winres.json --out cmd\compass-web\rsrc --arch amd64,386 >nul
    if errorlevel 1 echo [WARN] go-winres fallo; se compila sin recursos.
) else (
    echo [AVISO] go-winres no encontrado: se compila sin icono ni version.
)
REM App GUI de Windows: -H=windowsgui evita que se abra una consola al arrancar.
REM Las tareas --task enganchan la consola padre en main.go (AttachParentConsole)
REM para seguir escribiendo en la ConPTY del terminal embebido.
echo [INFO] Compilando frontend Svelte...
pushd frontend
call npm run build
if errorlevel 1 ( popd & echo [ERROR] Fallo el build de Svelte. & exit /b 1 )
popd
echo [OK] Frontend compilado.

go build -ldflags="-H=windowsgui" -o "dist\payload\compass-go.exe" ./cmd/compass-web
if errorlevel 1 (
    echo.
    echo =============================================
    echo   [ERROR] La compilacion fallo.
    echo =============================================
    echo.
    pause
    exit /b 1
)
echo [OK] dist\payload\compass-go.exe generado.

REM Iconos junto al ejecutable (el instalador los empaqueta tal cual).
for %%I in (CompassGo.ico VibeCoding.ico Git.ico Default.ico) do (
    if exist "assets\icon\%%I" copy /Y "assets\icon\%%I" "dist\payload\%%I" >nul 2>nul
)
if not exist "dist\payload\CompassGo.ico" if exist "dist\payload\Default.ico" (
    copy /Y "dist\payload\Default.ico" "dist\payload\CompassGo.ico" >nul 2>nul
)
if not exist "dist\payload\VibeCoding.ico" if exist "dist\payload\Default.ico" (
    copy /Y "dist\payload\Default.ico" "dist\payload\VibeCoding.ico" >nul 2>nul
)
if not exist "dist\payload\Git.ico" if exist "dist\payload\Default.ico" (
    copy /Y "dist\payload\Default.ico" "dist\payload\Git.ico" >nul 2>nul
)
echo [OK] Iconos copiados.
echo.

REM Core kits (motores canonicos, p.ej. whatsapp) viajan dentro del payload;
REM el instalador ya empaqueta payload\* con File /r.
REM
REM Se excluyen los artefactos de BUILD del motor (node_modules, out, examples):
REM en tiempo de ejecucion solo hacen falta node.exe + whatsapp-engine-bundle,
REM que ya viene compilado y es autocontenido. Esto reduce el payload de
REM ~2418 archivos a ~30 y la instalacion de ~15 s a unos pocos segundos.
REM Se borra la copia previa porque /MIR + /XD no purga las carpetas excluidas.
if exist "corekit" (
    echo [INFO] Copiando core kits a dist\payload sin artefactos de build...
    if exist "dist\payload\corekit" rmdir /s /q "dist\payload\corekit"
    robocopy "corekit" "dist\payload\corekit" /E /XD node_modules out examples /NFL /NDL /NJH /NJS /NP >nul
    if errorlevel 8 (
        echo [WARN] robocopy devolvio un error al copiar corekit; revisa dist\payload\corekit.
    ) else (
        echo [OK] Core kits copiados.
    )
) else (
    echo [SKIP] No se encontro la carpeta corekit; se omite.
)
echo.

if "%MAKE_INSTALLER%"=="0" (
    echo [SKIP] Instalador omitido por --no-installer. dist\payload queda listo.
    goto done
)

REM ---------------------------------------------------------------
REM  2. Generar el instalador autonomo en dist\
REM ---------------------------------------------------------------
echo [INFO] Preparando recursos del instalador...

REM Cabecera grafica del asistente (150x57 BMP) a partir del icono de la app.
if exist "dist\installer_header.bmp" del /f /q "dist\installer_header.bmp" >nul 2>nul
set "HEADER_ICON=assets\icon\CompassGo.ico"
if not exist "%HEADER_ICON%" set "HEADER_ICON=assets\icon\Icon.ico"
if exist "packaging\make_header.ps1" (
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "packaging\make_header.ps1" -Icon "%HEADER_ICON%" -Out "dist\installer_header.bmp" >nul 2>nul
)
set "HEADER_FLAG="
if exist "dist\installer_header.bmp" (
    echo [OK] Cabecera del asistente generada.
) else (
    echo [WARN] No se pudo generar la cabecera; se usara la cabecera estandar.
    set "HEADER_FLAG=/DNOHEADER"
)

REM Icono de marca para el asistente y el propio .exe del instalador.
set "ICON_FLAG="
if exist "dist\payload\CompassGo.ico" (
    set "ICON_FLAG=/DAPPICON=payload\CompassGo.ico"
) else if exist "dist\payload\Default.ico" (
    set "ICON_FLAG=/DAPPICON=payload\Default.ico"
)

set "FAST_FLAG="
if "%FAST%"=="1" (
    set "FAST_FLAG=/DFAST"
    echo [INFO] Compresion rapida zlib: instalador mayor pero mucho mas rapido.
)

REM dist\installer.nsi se copia de la plantilla versionada en cada ejecucion:
REM dist\ es desechable.
if not exist "packaging\installer.nsi" (
    echo [ERROR] Falta packaging\installer.nsi.
    goto done
)
copy /Y "packaging\installer.nsi" "dist\installer.nsi" >nul
if errorlevel 1 (
    echo [ERROR] No se pudo copiar packaging\installer.nsi a dist\.
    goto done
)
echo [OK] dist\installer.nsi listo.

if not exist "%MAKENSIS%" (
    echo [WARN] NSIS no esta instalado; se omite el instalador.
    echo        Instala NSIS desde https://nsis.sourceforge.io/ y ejecuta:
    echo        "%MAKENSIS%" %ICON_FLAG% %HEADER_FLAG% %FAST_FLAG% "dist\installer.nsi"
    goto done
)

echo [INFO] Generando dist\CompassGo-Setup.exe...
pushd "dist"
"%MAKENSIS%" %ICON_FLAG% %HEADER_FLAG% %FAST_FLAG% installer.nsi >nul
if errorlevel 1 (
    popd
    echo [ERROR] No se pudo generar el instalador.
    echo.
    exit /b 1
) else (
    popd
    echo [OK] dist\CompassGo-Setup.exe generado.
)
echo.

:done
echo =============================================
echo   [OK] dist listo
echo =============================================
echo   Payload:    %CD%\dist\payload\compass-go.exe
if "%MAKE_INSTALLER%"=="1" if exist "dist\CompassGo-Setup.exe" echo   Instalador: %CD%\dist\CompassGo-Setup.exe
echo.
exit /b 0
