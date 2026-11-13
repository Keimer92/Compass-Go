@echo off
setlocal EnableDelayedExpansion
title Desinstalando Compass Go...
echo.
echo =============================================
echo   Compass Go - Desinstalador
echo =============================================
echo.
echo Se eliminaran:
echo   - Los menus contextuales de Compass y .gitignore
echo   - La cascada de Vibe Coding
echo   - compass-go.exe e iconos de %LOCALAPPDATA%\Programs\Keimer
echo   - Restos antiguos en %USERPROFILE%\.cargo\bin
echo.
echo Las CLIs de Vibe Coding instaladas (opencode, cline...) NO se tocan.
echo.

set "BINDIR=%LOCALAPPDATA%\Programs\Keimer"
set "LEGACYBIN=%USERPROFILE%\.cargo\bin"

REM Con el binario aun presente se quitan las claves del registro: es el mismo
REM codigo que las creo, asi que no hay listas de claves duplicadas aqui.
set "EXE=%BINDIR%\compass-go.exe"
if not exist "%EXE%" set "EXE=%LEGACYBIN%\compass-go.exe"

if exist "%EXE%" (
    tasklist /fi "imagename eq compass-go.exe" 2>nul | "%SystemRoot%\System32\find.exe" /i "compass-go.exe" >nul
    if not errorlevel 1 (
        echo [INFO] Cerrando compass-go.exe en ejecucion...
        taskkill /f /im compass-go.exe >nul 2>nul
        timeout /t 1 /nobreak >nul 2>nul
    )

    echo [INFO] Quitando menu contextual de Compass...
    "%EXE%" --task context-menu-uninstall
    if errorlevel 1 (
        echo [WARN] No se pudo quitar el menu de Compass.
    ) else (
        echo [OK] Menu de Compass eliminado.
    )
    echo.

    echo [INFO] Quitando Vibe Coding...
    "%EXE%" --task opencode-uninstall
    if errorlevel 1 (
        echo [WARN] No se pudo quitar la cascada de Vibe Coding.
    ) else (
        echo [OK] Vibe Coding eliminado.
    )
    echo.
) else (
    echo [WARN] No se encontro compass-go.exe instalado.
    echo        Se omite la limpieza del registro.
    echo.
)

REM ---------------------------------------------------------------
REM  Quitar %BINDIR% del PATH del usuario (conservando REG_EXPAND_SZ)
REM ---------------------------------------------------------------
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$d='%BINDIR%'; $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment',$true); if($k){$v=[string]$k.GetValue('Path','',[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); $parts=@($v -split ';' | Where-Object { $_ -ne '' -and $_ -ne $d }); $k.SetValue('Path',($parts -join ';'),[Microsoft.Win32.RegistryValueKind]::ExpandString); $k.Close()}"
if errorlevel 1 (
    echo [WARN] No se pudo actualizar el PATH del usuario.
) else (
    echo [OK] PATH del usuario actualizado.
)
echo.

REM ---------------------------------------------------------------
REM  Quitar COMPASS_WHATSAPP_ENGINE del entorno del usuario
REM ---------------------------------------------------------------
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "[Environment]::SetEnvironmentVariable('COMPASS_WHATSAPP_ENGINE',$null,'User')"
if errorlevel 1 (
    echo [WARN] No se pudo quitar COMPASS_WHATSAPP_ENGINE.
) else (
    echo [OK] COMPASS_WHATSAPP_ENGINE eliminada.
)
echo.

echo [INFO] Borrando binario e iconos...
if exist "%BINDIR%\compass-go.exe" del /f /q "%BINDIR%\compass-go.exe" >nul 2>nul
if exist "%BINDIR%\Icon.ico"       del /f /q "%BINDIR%\Icon.ico" >nul 2>nul
if exist "%BINDIR%\Default.ico"    del /f /q "%BINDIR%\Default.ico" >nul 2>nul
if exist "%BINDIR%\opencode.ico"   del /f /q "%BINDIR%\opencode.ico" >nul 2>nul
REM Solo el .exe: los .ico de .cargo\bin los comparten otros tools.
if exist "%LEGACYBIN%\compass-go.exe" del /f /q "%LEGACYBIN%\compass-go.exe" >nul 2>nul
REM Borrar la carpeta Keimer si quedo vacia (no falla si no lo esta).
if exist "%BINDIR%" rmdir "%BINDIR%" >nul 2>nul
echo [OK] Archivos borrados.
echo.

ie4uinit.exe -show >nul 2>nul

echo =============================================
echo   [OK] Desinstalacion completada
echo =============================================
echo.
exit /b 0
