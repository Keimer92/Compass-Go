@echo off
setlocal
title Instalando Compass Go...
echo.
echo =============================================
echo   Compass Go - Compilar + Instalar + Empaquetar
echo =============================================
echo.

REM ---------------------------------------------------------------
REM  Flags:  install.bat --force         rehace tambien los menus y la cascada
REM          install.bat --vibe          vuelve a configurar Vibe Coding
REM          install.bat --no-installer  omite solo el setup (dist\payload se usa igual)
REM ---------------------------------------------------------------
set "FORCE=0"
set "WITH_VIBE=0"
set "MAKE_INSTALLER=1"
:parse
if "%~1"=="" goto parsed
if /i "%~1"=="--force"        set "FORCE=1"
if /i "%~1"=="-f"             set "FORCE=1"
if /i "%~1"=="--vibe"         set "WITH_VIBE=1"
if /i "%~1"=="--no-installer" set "MAKE_INSTALLER=0"
if /i "%~1"=="--no-dist"      set "MAKE_INSTALLER=0"
shift
goto parse
:parsed

set "BINDIR=%LOCALAPPDATA%\Programs\Keimer"
set "LEGACYBIN=%USERPROFILE%\.cargo\bin"
set "MAKENSIS=%ProgramFiles(x86)%\NSIS\makensis.exe"

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
REM  1. Compilar directo a dist\payload (sin artefactos en la raiz)
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
    echo La instalacion actual no se ha tocado.
    echo.
    pause
    exit /b 1
)
REM Limpiar el binario portable de versiones anteriores en la raiz.
if exist "compass-go.exe" del /f /q "compass-go.exe" >nul 2>nul
echo [OK] dist\payload\compass-go.exe generado.
echo.

REM ---------------------------------------------------------------
REM  2. Copiar binario e iconos
REM ---------------------------------------------------------------
if not exist "%BINDIR%" mkdir "%BINDIR%"

echo [INFO] Instalando en %BINDIR%...
copy /Y "dist\payload\compass-go.exe" "%BINDIR%\compass-go.exe" >nul
if errorlevel 1 (
    echo [ERROR] No se pudo copiar dist\payload\compass-go.exe a %BINDIR%.
    pause
    exit /b 1
)
echo [OK] compass-go.exe instalado.

REM Los iconos se resuelven junto al ejecutable (icon.ico / Default.ico /
REM opencode.ico); sin ellos el menu contextual muestra el icono generico.
REM Se copian a la instalacion y se dejan en dist\payload para el instalador.
for %%I in (Icon.ico Default.ico opencode.ico) do (
    if exist "assets\icon\%%I" (
        copy /Y "assets\icon\%%I" "%BINDIR%\%%I" >nul 2>nul
        copy /Y "assets\icon\%%I" "dist\payload\%%I" >nul 2>nul
    )
)
if not exist "%BINDIR%\Default.ico" if exist "%BINDIR%\Icon.ico" (
    copy /Y "%BINDIR%\Icon.ico" "%BINDIR%\Default.ico" >nul 2>nul
)
if not exist "%BINDIR%\opencode.ico" if exist "%BINDIR%\Default.ico" (
    copy /Y "%BINDIR%\Default.ico" "%BINDIR%\opencode.ico" >nul 2>nul
)
echo [OK] Iconos copiados.
echo.

REM ---------------------------------------------------------------
REM  3. Migrar la instalacion antigua en .cargo\bin
REM ---------------------------------------------------------------
REM Compass vivia antes en .cargo\bin (carpeta de Rust). Se borra solo
REM compass-go.exe; los .ico se dejan porque otros tools (code-runner) los
REM comparten.
if exist "%LEGACYBIN%\compass-go.exe" (
    del /f /q "%LEGACYBIN%\compass-go.exe" >nul 2>nul
    if exist "%LEGACYBIN%\compass-go.exe" (
        echo [WARN] No se pudo borrar %LEGACYBIN%\compass-go.exe.
    ) else (
        echo [OK] Copia antigua de .cargo\bin eliminada.
    )
)
echo.

REM ---------------------------------------------------------------
REM  4. Asegurar Compass y los shims globales de npm en el PATH del usuario
REM ---------------------------------------------------------------
REM Se edita HKCU\Environment\Path conservando el tipo REG_EXPAND_SZ y sin
REM expandir variables existentes (%USERPROFILE%, etc.).
powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$dirs=@('%BINDIR%'); $npm=Join-Path $env:APPDATA 'npm'; if(Test-Path -LiteralPath $npm -PathType Container){$dirs+=$npm}; $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment',$true); if($k){$v=[string]$k.GetValue('Path','',[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); foreach($d in $dirs){$paths=@($v -split ';' | ForEach-Object {$_.TrimEnd('\')}); if($paths -notcontains $d.TrimEnd('\')){if($v.TrimEnd(';') -eq ''){$v=$d}else{$v=$v.TrimEnd(';')+';'+$d}}}; $kind=try{$k.GetValueKind('Path')}catch{[Microsoft.Win32.RegistryValueKind]::ExpandString}; $k.SetValue('Path',$v,$kind); $k.Close()}"
if errorlevel 1 (
    echo [WARN] No se pudo actualizar el PATH del usuario.
    echo        Anade manualmente: %BINDIR% y %APPDATA%\npm
) else (
    echo [OK] PATH del usuario actualizado. Abre una terminal nueva para verlo.
)
echo.

REM ---------------------------------------------------------------
REM  5. Menu contextual de Compass
REM ---------------------------------------------------------------
echo [INFO] Configurando menu contextual de Compass...
"%BINDIR%\compass-go.exe" --task context-menu-install
if errorlevel 1 (
    echo [WARN] No se pudo instalar el menu contextual de Compass.
) else (
    echo [OK] Menu contextual de Compass instalado.
)
echo.

REM ---------------------------------------------------------------
REM  6. Vibe Coding (solo la primera vez o con --vibe)
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

REM ---------------------------------------------------------------
REM  7. Generar el instalador autonomo en dist\
REM ---------------------------------------------------------------
REM dist\payload (binario + iconos) ya se genero en los pasos 1-2.
echo [INFO] Preparando dist...

REM dist\installer.nsi se regenera en cada ejecucion: dist\ es desechable.
echo [INFO] Generando dist\installer.nsi...
> "dist\installer.nsi" echo ; ============================================================================
>> "dist\installer.nsi" echo ;  Compass Go - Instalador autonomo (NSIS)
>> "dist\installer.nsi" echo ;  Generado por install.bat. No editar a mano: se sobrescribe en cada build.
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

if "%MAKE_INSTALLER%"=="0" (
    echo [SKIP] Setup omitido por --no-installer. dist\payload e installer.nsi quedan listos.
    goto done
)

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
REM El icono de las entradas nuevas solo aparece al refrescar la cache.
ie4uinit.exe -show >nul 2>nul

echo =============================================
echo   [OK] Instalacion completada
echo =============================================
echo   Binario:    %BINDIR%\compass-go.exe
if "%MAKE_INSTALLER%"=="1" if exist "dist\CompassGo-Setup.exe" echo   Instalador: %CD%\dist\CompassGo-Setup.exe
echo.
echo Si el menu no aparece, reinicia el Explorador:
echo   taskkill /f /im explorer.exe ^& start explorer.exe
echo.
exit /b 0
