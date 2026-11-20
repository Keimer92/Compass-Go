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

echo [INFO] Esperando a que Vite escuche en 5173...
set "TRIES=0"
:wait
powershell -NoProfile -Command "try{$c=New-Object Net.Sockets.TcpClient;$c.Connect('127.0.0.1',5173);$c.Close();exit 0}catch{exit 1}"
if not errorlevel 1 goto ready
set /a TRIES+=1
if %TRIES% GEQ 40 (
    echo [ERROR] Vite no respondio en 20 s. Revisa la ventana "Compass Go - Vite".
    echo        ^(puerto ocupado^? cierra el proceso que lo use^)
    pause
    exit /b 1
)
timeout /t 1 /nobreak >nul
goto wait

:ready
echo [INFO] Compilando y abriendo Compass Go...
echo        Los cambios en la UI se recargan solos; los de Go requieren
echo        cerrar esta ventana y volver a ejecutar dev.bat.
echo.
go run ./cmd/compass-web --dev %*

echo.
echo [INFO] Compass Go cerrado. La ventana de Vite sigue abierta: cierrala a mano.
pause
