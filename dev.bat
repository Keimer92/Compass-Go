@echo off
setlocal
cd /d "%~dp0"
title Compass Go - desarrollo

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

REM Si ya hay un Vite vivo de una ejecucion anterior se reutiliza: con
REM strictPort un segundo `npm run dev` abortaria y su ventana se cerraria
REM dejando un parpadeo sin explicacion.
call :probe
if not errorlevel 1 (
    echo [INFO] Ya hay un servidor escuchando en 127.0.0.1:5173.
    echo        Se reutiliza: no se abre otra ventana de Vite.
    goto ready
)

if not exist "frontend\node_modules" (
    echo [INFO] Instalando dependencias del frontend...
    pushd frontend
    call npm install
    popd
)

REM Vite se queda en su propia ventana: si se cerrara, el WebSocket HMR y la
REM carga de modulos se cortarian y la UI quedaria en blanco.
echo [INFO] Arrancando Vite en una ventana aparte ^(puerto 5173^)...
start "Compass Go - Vite" /d "%~dp0frontend" cmd /k npm run dev

echo [INFO] Esperando a que Vite escuche en 127.0.0.1:5173...
set "TRIES=0"
:wait
call :probe
if not errorlevel 1 goto ready
set /a TRIES+=1
<nul set /p "=."
if %TRIES% GEQ 30 (
    echo.
    echo [ERROR] Vite no respondio en 30 s. Revisa la ventana "Compass Go - Vite".
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
echo        UI (Svelte/CSS/TS): HMR, se recarga sola.
echo        Go (backend): se recompila y reinicia al guardar.
echo        Al reiniciar por un cambio de Go se pierden las pestanas ConPTY.
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0dev-watch.ps1" %*

echo.
echo [INFO] Compass Go cerrado. La ventana de Vite sigue abierta: cierrala a mano.
pause
exit /b 0

:probe
powershell -NoProfile -Command "try{$c=New-Object Net.Sockets.TcpClient;$c.Connect('127.0.0.1',5173);$c.Close();exit 0}catch{exit 1}"
exit /b %errorlevel%
