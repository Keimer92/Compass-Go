; ============================================================================
;  Compass Go - Instalador autonomo (NSIS)
;  Generado por dist.bat. No editar a mano: se sobrescribe en cada build.
;
;  Instala compass-go.exe + iconos en LOCALAPPDATA\Programs\Keimer,
;  anade ese directorio al PATH del usuario y registra el menu contextual.
; ============================================================================

!define APPNAME  "Compass Go"
!define APPEXE   "compass-go.exe"
!define REGKEY   "CompassGo"
!define UNREGKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${REGKEY}"

Name "${APPNAME}"
OutFile "CompassGo-Setup.exe"
InstallDir "$LOCALAPPDATA\Programs\Keimer"
RequestExecutionLevel user

!include "WinMessages.nsh"
!include "StrFunc.nsh"
${StrStr}
${StrRep}
${UnStrStr}
${UnStrRep}

Section "Principal" SEC_PRINCIPAL
  SetOutPath "$INSTDIR"
  File /r "payload\*"

  ; Migracion: eliminar la copia antigua de .cargo\bin.
  Delete "$PROFILE\.cargo\bin\compass-go.exe"

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

  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "${UNREGKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKCU "${UNREGKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNREGKEY}" "DisplayIcon" "$INSTDIR\${APPEXE},0"
  WriteRegStr HKCU "${UNREGKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNREGKEY}" "Publisher" "${APPNAME}"
  WriteRegStr HKCU "${UNREGKEY}" "Version" "1.0.0.0"
  WriteRegDWORD HKCU "${UNREGKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNREGKEY}" "NoRepair" 1

  ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-install'
SectionEnd

Section "Uninstall"
  ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-uninstall'

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
