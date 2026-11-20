@echo off
setlocal
title Instalando Compass Go...
echo.
echo =============================================
echo   Compass Go - Compilar + Instalar
echo =============================================
echo.

REM ---------------------------------------------------------------
REM  Flags:  install.bat --force     rehace tambien la cascada Vibe Coding
REM          install.bat --vibe      vuelve a configurar Vibe Coding
REM          install.bat --with-node incluye node.exe (sidecar) en el payload
REM          install.bat --detached  (interno) ya se relanzo fuera de Compass
REM
REM  Por defecto NO se empaqueta node.exe: los proyectos usan el Node 20+
REM  instalado en el sistema (PATH). Usa --with-node para volver al sidecar.
REM
REM  Flujo: dist.bat compila y genera dist\CompassGo-Setup.exe, el instalador
REM  NSIS hace la instalacion real y al terminar se borra dist\ (solo dist.bat
REM  vuelve a crearlo).
REM ---------------------------------------------------------------
set "FORCE=0"
set "WITH_VIBE=0"
set "NO_NODE=1"
set "CG_DETACHED=0"
:parse
if "%~1"=="" goto parsed
if /i "%~1"=="--force"     set "FORCE=1"
if /i "%~1"=="-f"          set "FORCE=1"
if /i "%~1"=="--vibe"      set "WITH_VIBE=1"
if /i "%~1"=="--no-node"   set "NO_NODE=1"
if /i "%~1"=="--with-node" set "NO_NODE=0"
if /i "%~1"=="--detached"  set "CG_DETACHED=1"
shift
goto parse
:parsed

REM ---------------------------------------------------------------
REM  Guardia: si esto corre dentro de una pestana de Compass Go (las
REM  ConPTY exportan COMPASS_EMBEDDED=1), el taskkill de mas abajo cerraria
REM  la app y, con ella, esta consola y todos sus hijos: la instalacion
REM  abortaria a medias y el agente que la lanzo perderia su tarea.
REM  En ese caso se relanza en una consola aparte y se sale de inmediato.
REM ---------------------------------------------------------------
if "%COMPASS_EMBEDDED%"=="1" if not "%CG_DETACHED%"=="1" (
    echo [INFO] Detectada una pestana de Compass Go.
    echo        La instalacion cierra Compass, asi que continuara en una
    echo        consola aparte para no cortar esta tarea.
    echo.
    REM La consola nueva es normal: se limpia la marca para que no herede
    REM los ajustes de una pestana incrustada (titulo, cierre de pestana...).
    set "COMPASS_EMBEDDED="
    start "Compass Go - Instalacion" cmd /c call "%~f0" --detached %*
    exit /b 0
)

set "BINDIR=%LOCALAPPDATA%\Programs\Keimer"
set "SETUP=dist\CompassGo-Setup.exe"

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
REM  1. Compilar y empaquetar con dist.bat
REM ---------------------------------------------------------------
if exist "%SETUP%" del /f /q "%SETUP%" >nul 2>nul
set "DIST_FLAGS=--fast --no-node"
if "%NO_NODE%"=="0" set "DIST_FLAGS=--fast --with-node"
echo [INFO] Compilando y generando el instalador (dist.bat %DIST_FLAGS%)...
call "%~dp0dist.bat" %DIST_FLAGS%
if errorlevel 1 (
    echo [ERROR] dist.bat fallo; no se ha instalado nada.
    REM En la consola desacoplada la ventana se cerraria sin dejar leer el error.
    if "%CG_DETACHED%"=="1" pause
    exit /b 1
)
if not exist "%SETUP%" (
    echo [ERROR] No se genero %SETUP%.
    echo         NSIS es necesario: winget install NSIS.NSIS
    echo.
    pause
    exit /b 1
)

REM ---------------------------------------------------------------
REM  2. Instalar en silencio con el instalador NSIS
REM ---------------------------------------------------------------
echo [INFO] Instalando en silencio con %SETUP%...
start /wait "" "%SETUP%" /S
if not exist "%BINDIR%\compass-go.exe" (
    echo [ERROR] El instalador no dejo compass-go.exe en %BINDIR%.
    echo.
    pause
    exit /b 1
)
echo [OK] Binario, iconos, corekits, PATH, menu contextual y accesos instalados.
echo.

REM ---------------------------------------------------------------
REM  3. Variable COMPASS_WHATSAPP_ENGINE (motor canonico)
REM ---------------------------------------------------------------
REM Apunta a los proyectos que consumen el motor (p.ej. GymPro en modo
REM desarrollo) al engine canonico instalado junto al binario, para que lo
REM reutilicen sin copiarlo dentro del proyecto.
if exist "%BINDIR%\corekit\whatsapp" (
    echo [INFO] Configurando COMPASS_WHATSAPP_ENGINE...
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "[Environment]::SetEnvironmentVariable('COMPASS_WHATSAPP_ENGINE','%BINDIR%\corekit\whatsapp','User')"
    if errorlevel 1 (
        echo [WARN] No se pudo definir COMPASS_WHATSAPP_ENGINE.
        echo        Definela manualmente: %BINDIR%\corekit\whatsapp
    ) else (
        echo [OK] COMPASS_WHATSAPP_ENGINE = %BINDIR%\corekit\whatsapp
    )
) else (
    echo [SKIP] No se encontro corekit\whatsapp; se omite COMPASS_WHATSAPP_ENGINE.
)
echo.

REM ---------------------------------------------------------------
REM  4. Vibe Coding (solo la primera vez o con --vibe / --force)
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

REM Cache de iconos: si CompassGo.ico cambio respecto a la instalacion anterior,
REM se reconstruye (reinicia Explorer un momento); si no, refresco suave.
REM Sin esto, el acceso del escritorio / menu Inicio / barra de tareas puede
REM seguir pintando el icono viejo aunque el .exe ya lleve el nuevo.
if exist "%~dp0packaging\refresh_icons.ps1" (
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0packaging\refresh_icons.ps1" -IconPath "%BINDIR%\CompassGo.ico" -StampPath "%LOCALAPPDATA%\compass\icon-stamp.txt"
) else (
    ie4uinit.exe -show >nul 2>nul
)

REM dist.bat ya borra dist\payload al empaquetar; este barrido es solo una
REM red de seguridad (p.ej. si dist.bat fallo antes de limpiar).
REM El instalador autocontenido dist\CompassGo-Setup.exe se CONSERVA para
REM poder distribuirlo o reinstalar sin volver a compilar.
if exist "%CD%\dist\payload" (
    rmdir /s /q "%CD%\dist\payload" >nul 2>nul
    echo [OK] dist\payload eliminado.
)
if exist "%SETUP%" (
    echo [OK] Instalador autocontenido conservado: %CD%\%SETUP%
) else (
    echo [SKIP] No se encontro %SETUP%.
)
echo.

echo =============================================
echo   [OK] Instalacion completada
echo =============================================
echo   Binario:   %BINDIR%\compass-go.exe
if exist "%SETUP%" echo   Instalador: %CD%\%SETUP%
echo.
echo Abre Compass Go desde el menu Inicio (o ejecuta compass-go).
echo Si quieres anclarlo: menu Inicio ^> clic derecho ^> Anclar a la barra de tareas.
echo.
REM En la consola desacoplada se deja el resumen a la vista antes de cerrar.
if "%CG_DETACHED%"=="1" pause
exit /b 0
