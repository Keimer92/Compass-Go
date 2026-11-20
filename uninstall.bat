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
echo   - compass-go.exe, iconos y corekits de %LOCALAPPDATA%\Programs\Keimer
echo   - Los accesos directos y la entrada de "Aplicaciones instaladas"
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
REM  Desinstalador del instalador NSIS (accesos directos, PATH, registro)
REM ---------------------------------------------------------------
if exist "%BINDIR%\Uninstall.exe" (
    echo [INFO] Ejecutando el desinstalador de la instalacion...
    start /wait "" "%BINDIR%\Uninstall.exe" /S

    REM El desinstalador se copia a %%TEMP%% y puede volver antes de terminar:
    REM se espera (max. 15 s) a que desaparezca el binario.
    call :wait_uninstall

    if exist "%BINDIR%\compass-go.exe" (
        echo [WARN] El desinstalador no termino; se limpia el resto a mano.
    ) else (
        echo [OK] Desinstalador ejecutado.
    )
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

REM ---------------------------------------------------------------
REM  Limpieza a mano (idempotente: cubre instalaciones sin NSIS)
REM ---------------------------------------------------------------
echo [INFO] Borrando binario, iconos y corekits...
if exist "%BINDIR%\compass-go.exe" del /f /q "%BINDIR%\compass-go.exe" >nul 2>nul
if exist "%BINDIR%\CompassGo.ico"  del /f /q "%BINDIR%\CompassGo.ico" >nul 2>nul
if exist "%BINDIR%\VibeCoding.ico" del /f /q "%BINDIR%\VibeCoding.ico" >nul 2>nul
if exist "%BINDIR%\Git.ico"        del /f /q "%BINDIR%\Git.ico" >nul 2>nul
if exist "%BINDIR%\Uninstall.exe"  del /f /q "%BINDIR%\Uninstall.exe" >nul 2>nul
if exist "%BINDIR%\corekit"        rmdir /s /q "%BINDIR%\corekit" >nul 2>nul
REM Solo el .exe: los .ico de .cargo\bin los comparten otros tools.
if exist "%LEGACYBIN%\compass-go.exe" del /f /q "%LEGACYBIN%\compass-go.exe" >nul 2>nul
REM Borrar la carpeta Keimer si quedo vacia (no falla si no lo esta).
if exist "%BINDIR%" rmdir "%BINDIR%" >nul 2>nul
echo [OK] Archivos borrados.
echo.

REM El icono de los accesos ya borrados puede quedar en cache: se refresca y se
REM olvida el stamp para que la proxima instalacion reconstruya la cache.
if exist "%LOCALAPPDATA%\compass\icon-stamp.txt" del /f /q "%LOCALAPPDATA%\compass\icon-stamp.txt" >nul 2>nul
if exist "%~dp0packaging\refresh_icons.ps1" (
    powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0packaging\refresh_icons.ps1"
) else (
    ie4uinit.exe -show >nul 2>nul
)

echo =============================================
echo   [OK] Desinstalacion completada
echo =============================================
echo.
exit /b 0

:wait_uninstall
set /a tries=0
:wait_uninstall_loop
if not exist "%BINDIR%\compass-go.exe" exit /b 0
set /a tries+=1
if %tries% geq 15 exit /b 0
timeout /t 1 /nobreak >nul 2>nul
goto wait_uninstall_loop
