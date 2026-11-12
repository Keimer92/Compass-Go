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
REM ---------------------------------------------------------------
set "MAKE_INSTALLER=1"
:parse
if "%~1"=="" goto parsed
if /i "%~1"=="--no-installer" set "MAKE_INSTALLER=0"
if /i "%~1"=="--no-dist"      set "MAKE_INSTALLER=0"
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
REM  1. Compilar directo a dist\payload
REM ---------------------------------------------------------------
echo [INFO] Compilando en dist\payload...
if not exist "dist\payload" mkdir "dist\payload"
where go-winres >nul 2>nul
if not errorlevel 1 (
    echo [INFO] Generando recursos de Windows con go-winres...
    go-winres make --in go-winres.json --out cmd\compass\rsrc --arch amd64,386 >nul
    if errorlevel 1 echo [WARN] go-winres fallo; se compila sin recursos.
) else (
    echo [AVISO] go-winres no encontrado: se compila sin icono ni version.
)
go build -o "dist\payload\compass-go.exe" ./cmd/compass
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
for %%I in (Icon.ico Default.ico opencode.ico) do (
    if exist "assets\icon\%%I" copy /Y "assets\icon\%%I" "dist\payload\%%I" >nul 2>nul
)
if not exist "dist\payload\Default.ico" if exist "dist\payload\Icon.ico" (
    copy /Y "dist\payload\Icon.ico" "dist\payload\Default.ico" >nul 2>nul
)
if not exist "dist\payload\opencode.ico" if exist "dist\payload\Default.ico" (
    copy /Y "dist\payload\Default.ico" "dist\payload\opencode.ico" >nul 2>nul
)
echo [OK] Iconos copiados.
echo.

if "%MAKE_INSTALLER%"=="0" (
    echo [SKIP] Instalador omitido por --no-installer. dist\payload queda listo.
    goto done
)

REM ---------------------------------------------------------------
REM  2. Generar el instalador autonomo en dist\
REM ---------------------------------------------------------------
echo [INFO] Generando dist\installer.nsi...

REM dist\installer.nsi se regenera en cada ejecucion: dist\ es desechable.
> "dist\installer.nsi" echo ; ============================================================================
>> "dist\installer.nsi" echo ;  Compass Go - Instalador autonomo (NSIS)
>> "dist\installer.nsi" echo ;  Generado por dist.bat. No editar a mano: se sobrescribe en cada build.
>> "dist\installer.nsi" echo ;
>> "dist\installer.nsi" echo ;  Instala compass-go.exe + iconos en LOCALAPPDATA\Programs\Keimer,
>> "dist\installer.nsi" echo ;  anade ese directorio al PATH del usuario y registra el menu contextual.
>> "dist\installer.nsi" echo ; ============================================================================
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo !define APPNAME  "Compass Go"
>> "dist\installer.nsi" echo !define APPEXE   "compass-go.exe"
>> "dist\installer.nsi" echo !define REGKEY   "CompassGo"
>> "dist\installer.nsi" echo !define UNREGKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${REGKEY}"
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo Name "${APPNAME}"
>> "dist\installer.nsi" echo OutFile "CompassGo-Setup.exe"
>> "dist\installer.nsi" echo InstallDir "$LOCALAPPDATA\Programs\Keimer"
>> "dist\installer.nsi" echo RequestExecutionLevel user
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo !include "WinMessages.nsh"
>> "dist\installer.nsi" echo !include "StrFunc.nsh"
>> "dist\installer.nsi" echo ${StrStr}
>> "dist\installer.nsi" echo ${StrRep}
>> "dist\installer.nsi" echo ${UnStrStr}
>> "dist\installer.nsi" echo ${UnStrRep}
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo Section "Principal" SEC_PRINCIPAL
>> "dist\installer.nsi" echo   SetOutPath "$INSTDIR"
>> "dist\installer.nsi" echo   File /r "payload\*"
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo   ; Migracion: eliminar la copia antigua de .cargo\bin.
>> "dist\installer.nsi" echo   Delete "$PROFILE\.cargo\bin\compass-go.exe"
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo   ReadRegStr $0 HKCU "Environment" "Path"
>> "dist\installer.nsi" echo   ${StrStr} $1 $0 "$INSTDIR"
>> "dist\installer.nsi" echo   ${If} $1 == ""
>> "dist\installer.nsi" echo     ${If} $0 == ""
>> "dist\installer.nsi" echo       WriteRegExpandStr HKCU "Environment" "Path" "$INSTDIR;"
>> "dist\installer.nsi" echo     ${Else}
>> "dist\installer.nsi" echo       WriteRegExpandStr HKCU "Environment" "Path" "$0;$INSTDIR"
>> "dist\installer.nsi" echo     ${EndIf}
>> "dist\installer.nsi" echo     SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment"
>> "dist\installer.nsi" echo   ${EndIf}
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo   WriteUninstaller "$INSTDIR\Uninstall.exe"
>> "dist\installer.nsi" echo   WriteRegStr HKCU "${UNREGKEY}" "DisplayName" "${APPNAME}"
>> "dist\installer.nsi" echo   WriteRegStr HKCU "${UNREGKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
>> "dist\installer.nsi" echo   WriteRegStr HKCU "${UNREGKEY}" "DisplayIcon" "$INSTDIR\${APPEXE},0"
>> "dist\installer.nsi" echo   WriteRegStr HKCU "${UNREGKEY}" "InstallLocation" "$INSTDIR"
>> "dist\installer.nsi" echo   WriteRegStr HKCU "${UNREGKEY}" "Publisher" "${APPNAME}"
>> "dist\installer.nsi" echo   WriteRegStr HKCU "${UNREGKEY}" "Version" "1.0.0.0"
>> "dist\installer.nsi" echo   WriteRegDWORD HKCU "${UNREGKEY}" "NoModify" 1
>> "dist\installer.nsi" echo   WriteRegDWORD HKCU "${UNREGKEY}" "NoRepair" 1
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo   ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-install'
>> "dist\installer.nsi" echo SectionEnd
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo Section "Uninstall"
>> "dist\installer.nsi" echo   ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-uninstall'
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo   ReadRegStr $0 HKCU "Environment" "Path"
>> "dist\installer.nsi" echo   ${UnStrStr} $1 $0 "$INSTDIR"
>> "dist\installer.nsi" echo   ${If} $1 != ""
>> "dist\installer.nsi" echo     ${UnStrRep} $0 $0 "$INSTDIR;" ""
>> "dist\installer.nsi" echo     ${UnStrRep} $0 $0 ";$INSTDIR" ""
>> "dist\installer.nsi" echo     ${UnStrRep} $0 $0 "$INSTDIR" ""
>> "dist\installer.nsi" echo     WriteRegExpandStr HKCU "Environment" "Path" $0
>> "dist\installer.nsi" echo     SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment"
>> "dist\installer.nsi" echo   ${EndIf}
>> "dist\installer.nsi" echo.
>> "dist\installer.nsi" echo   Delete "$INSTDIR\Uninstall.exe"
>> "dist\installer.nsi" echo   RMDir /r "$INSTDIR"
>> "dist\installer.nsi" echo   DeleteRegKey HKCU "${UNREGKEY}"
>> "dist\installer.nsi" echo SectionEnd
echo [OK] dist\installer.nsi generado.

if not exist "%MAKENSIS%" (
    echo [WARN] NSIS no esta instalado; se omite el instalador.
    echo        Instala NSIS desde https://nsis.sourceforge.io/ y ejecuta:
    echo        "%MAKENSIS%" "dist\installer.nsi"
    goto done
)

echo [INFO] Generando dist\CompassGo-Setup.exe...
pushd "dist"
"%MAKENSIS%" installer.nsi >nul
if errorlevel 1 (
    popd
    echo [WARN] No se pudo generar el instalador.
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
