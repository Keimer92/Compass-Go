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
:parse
if "%~1"=="" goto parsed
if /i "%~1"=="--force"     set "FORCE=1"
if /i "%~1"=="-f"          set "FORCE=1"
if /i "%~1"=="--vibe"      set "WITH_VIBE=1"
if /i "%~1"=="--no-node"   set "NO_NODE=1"
if /i "%~1"=="--with-node" set "NO_NODE=0"
shift
goto parse
:parsed

REM ---------------------------------------------------------------
REM  Nota: install.bat YA NO cierra Compass Go. Windows no permite
REM  sobrescribir un ejecutable en uso, asi que si Compass esta abierto se
REM  aborta con un aviso para que lo cierres tu mismo. Asi no se matan las
REM  pestanas ni las CLIs que tengas abiertas dentro de Compass.
REM ---------------------------------------------------------------

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

REM Windows no deja sobrescribir un ejecutable en uso. NO matamos Compass Go:
REM si esta abierto se aborta para que lo cierres tu mismo y no pierdas tus
REM pestanas ni las CLIs que tengas corriendo dentro.
call :require_compass_closed
if errorlevel 1 exit /b 1
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
    pause
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
REM dist.bat tarda varios minutos: el usuario pudo abrir Compass mientras
REM compilaba. Se vuelve a comprobar antes de tocar el directorio instalado.
call :require_compass_closed
if errorlevel 1 exit /b 1
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
if exist "%BINDIR%\Kit-Whatsapp" (
    echo [INFO] Configurando COMPASS_WHATSAPP_ENGINE...
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "[Environment]::SetEnvironmentVariable('COMPASS_WHATSAPP_ENGINE','%BINDIR%\Kit-Whatsapp','User')"
    if errorlevel 1 (
        echo [WARN] No se pudo definir COMPASS_WHATSAPP_ENGINE.
        echo        Definela manualmente: %BINDIR%\Kit-Whatsapp
    ) else (
        echo [OK] COMPASS_WHATSAPP_ENGINE = %BINDIR%\Kit-Whatsapp
    )
) else (
    echo [SKIP] No se encontro "Kit-Whatsapp"; se omite COMPASS_WHATSAPP_ENGINE.
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
pause
exit /b 0

REM ---------------------------------------------------------------
REM  Comprueba si Compass Go esta en ejecucion. No lo cierra: solo avisa y
REM  devuelve 1 para que install.bat se detenga. El usuario cierra Compass a
REM  mano y conserva sus pestanas y las CLIs que tenga abiertas dentro.
REM ---------------------------------------------------------------
:require_compass_closed
REM find devuelve 0 solo si encontro la linea con el proceso; 1 = no esta
REM corriendo y se puede seguir.
tasklist /fi "imagename eq compass-go.exe" 2>nul | "%SystemRoot%\System32\find.exe" /i "compass-go.exe" >nul
if errorlevel 1 exit /b 0
echo [ERROR] Compass Go esta en ejecucion.
echo         Cierralo (guarda lo que necesites) y vuelve a ejecutar install.bat.
echo         No se ha cerrado ni modificado nada.
echo.
pause
exit /b 1
