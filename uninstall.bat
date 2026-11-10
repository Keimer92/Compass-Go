@echo off
setlocal EnableDelayedExpansion
title Desinstalando Compass...
echo.
echo =============================================
echo   Compass for Go - Desinstalador
echo =============================================
echo.
echo Se eliminaran:
echo   - Los menus contextuales de Compass y .gitignore
echo   - La cascada de Vibe Coding
echo   - compass-go.exe e iconos de %USERPROFILE%\.cargo\bin
echo.
echo Las CLIs de Vibe Coding instaladas (opencode, cline...) NO se tocan.
echo.

set "BINDIR=%USERPROFILE%\.cargo\bin"

REM Con el binario aun presente se quitan las claves del registro: es el mismo
REM codigo que las creo, asi que no hay listas de claves duplicadas aqui.
if exist "%BINDIR%\compass-go.exe" (
    tasklist /fi "imagename eq compass-go.exe" 2>nul | "%SystemRoot%\System32\find.exe" /i "compass-go.exe" >nul
    if not errorlevel 1 (
        echo [INFO] Cerrando compass-go.exe en ejecucion...
        taskkill /f /im compass-go.exe >nul 2>nul
        timeout /t 1 /nobreak >nul 2>nul
    )

    echo [INFO] Quitando menu contextual de Compass...
    "%BINDIR%\compass-go.exe" --task context-menu-uninstall
    if errorlevel 1 (
        echo [WARN] No se pudo quitar el menu de Compass.
    ) else (
        echo [OK] Menu de Compass eliminado.
    )
    echo.

    echo [INFO] Quitando Vibe Coding...
    "%BINDIR%\compass-go.exe" --task opencode-uninstall
    if errorlevel 1 (
        echo [WARN] No se pudo quitar la cascada de Vibe Coding.
    ) else (
        echo [OK] Vibe Coding eliminado.
    )
    echo.
) else (
    echo [WARN] No se encontro %BINDIR%\compass-go.exe.
    echo        Se omite la limpieza del registro.
    echo.
)

echo [INFO] Borrando binario e iconos...
if exist "%BINDIR%\compass-go.exe" del /f /q "%BINDIR%\compass-go.exe" >nul 2>nul
if exist "%BINDIR%\Icon.ico"     del /f /q "%BINDIR%\Icon.ico" >nul 2>nul
if exist "%BINDIR%\Default.ico"  del /f /q "%BINDIR%\Default.ico" >nul 2>nul
if exist "%BINDIR%\opencode.ico" del /f /q "%BINDIR%\opencode.ico" >nul 2>nul
echo [OK] Archivos borrados.
echo.

ie4uinit.exe -show >nul 2>nul

echo =============================================
echo   [OK] Desinstalacion completada
echo =============================================
echo.
exit /b 0
