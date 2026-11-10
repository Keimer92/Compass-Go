; ============================================================================
;  Compass Go - Instalador autónomo (NSIS)
;
;  Empaqueta compass-go.exe + iconos, los instala en
;  %LOCALAPPDATA%\Programs\Compass Go, añade el directorio al PATH del usuario
;  y registra el menú contextual de Windows. No requiere Go ni scripts sueltos.
; ============================================================================

!define APPNAME  "Compass Go"
!define APPEXE   "compass-go.exe"
!define REGKEY   "CompassGo"
!define UNREGKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${REGKEY}"

Name "${APPNAME}"
OutFile "CompassGo-Setup.exe"
InstallDir "$LOCALAPPDATA\Programs\${APPNAME}"
RequestExecutionLevel user

!include "WinMessages.nsh"
!include "StrFunc.nsh"
${StrStr}
${StrRep}
${UnStrStr}
${UnStrRep}

; --- Sección de instalación ---
Section "Principal" SEC_PRINCIPAL
  SetOutPath "$INSTDIR"
  File /r "payload\*"

  ; Añadir $INSTDIR al PATH del usuario (HKCU\Environment) si no está.
  ReadRegStr $0 HKCU "Environment" "Path"
  ${StrStr} $1 $0 "$INSTDIR"
  ${If} $1 == ""
    ${If} $0 == ""
      WriteRegExpandStr HKCU "Environment" "Path" "$INSTDIR;"
    ${Else}
      WriteRegExpandStr HKCU "Environment" "Path" "$0;$INSTDIR"
    ${EndIf}
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment"
  ${EndIf}

  ; Entrada de desinstalación en "Agregar o quitar programas".
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "${UNREGKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKCU "${UNREGKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNREGKEY}" "DisplayIcon" "$INSTDIR\${APPEXE},0"
  WriteRegStr HKCU "${UNREGKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNREGKEY}" "Publisher" "${APPNAME}"
  WriteRegStr HKCU "${UNREGKEY}" "Version" "1.0.0.0"
  WriteRegDWORD HKCU "${UNREGKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNREGKEY}" "NoRepair" 1

  ; Registrar el menú contextual de Windows (apunta a $INSTDIR\${APPEXE}).
  ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-install'
SectionEnd

; --- Sección de desinstalación ---
Section "Uninstall"
  ; Quitar el menú contextual (limpia también la clave antigua "zz_Compass").
  ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-uninstall'

  ; Quitar $INSTDIR del PATH del usuario.
  ReadRegStr $0 HKCU "Environment" "Path"
  ${UnStrStr} $1 $0 "$INSTDIR"
  ${If} $1 != ""
    ${UnStrRep} $0 $0 "$INSTDIR;" ""
    ${UnStrRep} $0 $0 ";$INSTDIR" ""
    ${UnStrRep} $0 $0 "$INSTDIR" ""
    WriteRegExpandStr HKCU "Environment" "Path" $0
    SendMessage ${HWND_BROADCAST} ${WM_WININICHANGE} 0 "STR:Environment"
  ${EndIf}

  Delete "$INSTDIR\Uninstall.exe"
  RMDir /r "$INSTDIR"
  DeleteRegKey HKCU "${UNREGKEY}"
SectionEnd
