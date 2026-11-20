@echo off
setlocal
cd /d "%~dp0"
REM El titulo se pone MAS ABAJO, despues de dev-reset.ps1: asi el script puede
REM cerrar la terminal del dev.bat anterior sin cerrarse a si mismo.

set "VITE_LOG=%LOCALAPPDATA%\compass\vite-dev.log"

echo.
echo =============================================
echo   Compass Go - modo desarrollo (HMR)
echo =============================================
echo.

where go >nul 2>nul
if errorlevel 1 (
    echo [ERROR] Go no esta en el PATH.
    echo         https://go.dev/dl/
    pause
    exit /b 1
)
where npm >nul 2>nul
if errorlevel 1 (
    echo [ERROR] Node/npm no estan en el PATH ^(se necesita Node 20+^).
    pause
    exit /b 1
)

REM Si ya habia una sesion de desarrollo, se cierra (Compass --dev + su
REM terminal) para que arranque una limpia.
echo [INFO] Cerrando sesion de desarrollo anterior si habia una...
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev-reset.ps1"
title Compass Go - desarrollo

REM Si ya hay un Vite vivo de una ejecucion anterior se reutiliza: con
REM strictPort un segundo `npm run dev` abortaria.
call :probe
if not errorlevel 1 (
    echo [INFO] Ya hay un servidor escuchando en 127.0.0.1:5173.
    echo        Se reutiliza: no se arranca otro Vite.
    goto ready
)
REM Margen para que el Vite del dev.bat anterior (que murio con su consola)
REM termine de soltar el puerto antes de arrancar otro.
timeout /t 1 /nobreak >nul
call :probe
if not errorlevel 1 goto ready

if not exist "frontend\node_modules" (
    echo [INFO] Instalando dependencias del frontend...
    pushd frontend
    call npm install
    popd
)

REM Vite corre en SEGUNDO PLANO en esta misma consola (start /b): no se abre
REM una segunda ventana y, al cerrar esta, Vite muere con ella.
echo [INFO] Arrancando Vite en segundo plano ^(puerto 5173^)...
echo        Su salida se escribe en %VITE_LOG%
start "Compass Go - Vite" /b "%~dp0frontend\dev-vite.bat"

echo [INFO] Esperando a que Vite escuche en 127.0.0.1:5173...
set "TRIES=0"
:wait
call :probe
if not errorlevel 1 goto ready
set /a TRIES+=1
<nul set /p "=."
if %TRIES% GEQ 30 (
    echo.
    echo [ERROR] Vite no respondio en 30 s. Ultimas lineas del log:
    powershell -NoProfile -Command "if (Test-Path -LiteralPath '%VITE_LOG%') { Get-Content -LiteralPath '%VITE_LOG%' -Tail 15 }" 2>nul
    echo        Si dice que el puerto esta ocupado, cierra ese proceso.
    echo        Si sigue sin responder, revisa que vite.config.ts tenga
    echo        server.host = '127.0.0.1' ^(por defecto escucha solo en IPv6^).
    pause
    exit /b 1
)
timeout /t 1 /nobreak >nul
goto wait

:ready
echo.
echo [OK] Vite listo en http://127.0.0.1:5173
echo [INFO] Compilando y abriendo Compass Go...
echo        Los cambios en la UI se recargan solos; los de Go requieren
echo        cerrar esta ventana y volver a ejecutar dev.bat.
echo.
go run ./cmd/compass-web --dev %*

echo.
echo [INFO] Compass Go cerrado. Vite sigue vivo en segundo plano: al cerrar
echo        esta ventana se detiene tambien.
pause
exit /b 0

:probe
powershell -NoProfile -Command "try{$c=New-Object Net.Sockets.TcpClient;$c.Connect('127.0.0.1',5173);$c.Close();exit 0}catch{exit 1}"
exit /b %errorlevel%
