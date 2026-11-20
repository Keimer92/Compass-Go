@echo off
REM Lanzado por dev.bat con `start /b`: comparte la consola del dev.bat y
REM muere con ella, asi no se abre una segunda ventana. La salida va a un log
REM para no mezclarse con la de `go run`.
setlocal
cd /d "%~dp0"
if "%VITE_LOG%"=="" set "VITE_LOG=%LOCALAPPDATA%\compass\vite-dev.log"
if not exist "%LOCALAPPDATA%\compass" mkdir "%LOCALAPPDATA%\compass" >nul 2>nul
npm run dev > "%VITE_LOG%" 2>&1
