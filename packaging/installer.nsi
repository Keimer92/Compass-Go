; ============================================================================
;  Compass Go - Instalador autonomo (NSIS / Modern UI 2)
;
;  Plantilla versionada en packaging\installer.nsi. dist.bat la copia a
;  dist\installer.nsi y compila con makensis. No editar la copia de dist\:
;  se sobrescribe en cada build.
;
;  Define opcionales que pasa dist.bat:
;    /DAPPICON=payload\CompassGo.ico   icono de marca (asistente + .exe)
;    /DNOHEADER                   omite la cabecera grafica si no se genero
; ============================================================================

Unicode true

!include "MUI2.nsh"
!include "WinMessages.nsh"
!include "StrFunc.nsh"

${StrStr}
${UnStrStr}
${UnStrRep}

!define APPNAME   "Compass Go"
!define APPEXE    "compass-go.exe"
!define APPVER    "1.0.0.0"
!define REGKEY    "CompassGo"
!define UNREGKEY  "Software\Microsoft\Windows\CurrentVersion\Uninstall\${REGKEY}"

Name "${APPNAME}"
OutFile "CompassGo-Setup.exe"
InstallDir "$LOCALAPPDATA\Programs\Keimer"
InstallDirRegKey HKCU "Software\${REGKEY}" "InstallDir"
RequestExecutionLevel user
; Compresion: LZMA lenta pero compacta para distribuir; zlib rapida para el
; ciclo de desarrollo (install.bat pasa /DFAST).
!ifdef FAST
  SetCompressor zlib
!else
  SetCompressor /SOLID lzma
  SetCompressorDictSize 64
!endif
BrandingText "${APPNAME} ${APPVER}"

VIProductVersion "${APPVER}"
VIAddVersionKey /LANG=1033 "ProductName"     "${APPNAME}"
VIAddVersionKey /LANG=1033 "FileDescription" "Instalador de ${APPNAME}"
VIAddVersionKey /LANG=1033 "FileVersion"     "${APPVER}"
VIAddVersionKey /LANG=1033 "ProductVersion"  "${APPVER}"
VIAddVersionKey /LANG=1033 "OriginalFilename" "CompassGo-Setup.exe"
VIAddVersionKey /LANG=1033 "LegalCopyright"  "Compass Go"

; ── Marca: icono de la app (como Firefox usa el suyo) ───────────────────────
!ifdef APPICON
  Icon          "${APPICON}"
  UninstallIcon "${APPICON}"
  !define MUI_ICON   "${APPICON}"
  !define MUI_UNICON "${APPICON}"
!endif

!ifndef NOHEADER
  !define MUI_HEADERIMAGE
  !define MUI_HEADERIMAGE_RIGHT
  !define MUI_HEADERIMAGE_BITMAP "installer_header.bmp"
  !define MUI_HEADERIMAGE_BITMAP_NOSTRETCH
!endif

; ── Textos del asistente ────────────────────────────────────────────────────
!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "Bienvenido a ${APPNAME}"
!define MUI_WELCOMEPAGE_TEXT "Este asistente instalara ${APPNAME} en tu equipo.$\r$\n$\r$\nPodras elegir acceso directo en el menu Inicio, en el escritorio y en la barra de tareas. Al final tambien podras ejecutar ${APPNAME} directamente.$\r$\n$\r$\nPulsa Siguiente para continuar."
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APPEXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Ejecutar ${APPNAME} ahora"
!define MUI_FINISHPAGE_TEXT "${APPNAME} se ha instalado correctamente.$\r$\n$\r$\nPuedes abrirlo desde el menu Inicio, el escritorio o ejecutando compass-go en una terminal."

; ── Paginas ─────────────────────────────────────────────────────────────────
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Spanish"

; ── Seccion principal (obligatoria) ─────────────────────────────────────────
Section "!${APPNAME} (obligatorio)" SEC_MAIN
  SectionIn RO

  SetOutPath "$INSTDIR"

  ; Limpieza de artefactos de build de versiones anteriores del motor de
  ; WhatsApp. El runtime usa el Node 20+ del sistema y el bundle ya viaja
  ; compilado; node_modules, out, examples y el sidecar node.exe ya no viajan.
  RMDir /r "$INSTDIR\corekit\whatsapp\node_modules"
  RMDir /r "$INSTDIR\corekit\whatsapp\out"
  RMDir /r "$INSTDIR\corekit\whatsapp\examples"
  ; Borra el sidecar de instalaciones anteriores para que no se prefiera sobre
  ; el Node del PATH.
  Delete "$INSTDIR\corekit\whatsapp\node.exe"

  File /r "payload\*"

  ; Migracion: eliminar la copia antigua de .cargo\bin.
  Delete "$PROFILE\.cargo\bin\compass-go.exe"

  ; Migracion: iconos retirados (Default.ico, Icon.ico, opencode.ico). Ya no
  ; viajan en el payload; se borran de instalaciones anteriores para no dejar
  ; restos con el icono viejo.
  Delete "$INSTDIR\Default.ico"
  Delete "$INSTDIR\Icon.ico"
  Delete "$INSTDIR\opencode.ico"

  ; PATH del usuario (conserva el tipo REG_EXPAND_SZ y el resto de entradas).
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

  WriteRegStr HKCU "Software\${REGKEY}" "InstallDir" "$INSTDIR"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "${UNREGKEY}" "DisplayName"     "${APPNAME}"
  WriteRegStr HKCU "${UNREGKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNREGKEY}" "DisplayIcon"     "$INSTDIR\${APPEXE},0"
  WriteRegStr HKCU "${UNREGKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNREGKEY}" "Publisher"       "${APPNAME}"
  WriteRegStr HKCU "${UNREGKEY}" "DisplayVersion"  "${APPVER}"
  WriteRegDWORD HKCU "${UNREGKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNREGKEY}" "NoRepair" 1

  ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-install'
SectionEnd

; ── Acceso directo en el menu Inicio ────────────────────────────────────────
Section "Acceso directo en el menu Inicio" SEC_STARTMENU
  CreateShortcut "$SMPROGRAMS\${APPNAME}.lnk" "$INSTDIR\${APPEXE}" "" "$INSTDIR\${APPEXE}" 0
SectionEnd

; ── Acceso directo en el escritorio ─────────────────────────────────────────
Section "Acceso directo en el escritorio" SEC_DESKTOP
  CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\${APPEXE}" "" "$INSTDIR\${APPEXE}" 0
SectionEnd

; ── Anclar a la barra de tareas (best-effort) ───────────────────────────────
; Windows 10 1903+ bloquea el verbo "taskbarpin" para instaladores. Se intenta
; igualmente y, si no queda anclado, se avisa de como hacerlo a mano.
Section "Anclar a la barra de tareas" SEC_PIN
  CreateShortcut "$SMPROGRAMS\${APPNAME}.lnk" "$INSTDIR\${APPEXE}" "" "$INSTDIR\${APPEXE}" 0

  ; Shell.Application.InvokeVerb no cae al verbo "open" si Windows no expone
  ; "taskbarpin"; ExecShell si lo haria y abriria la app al instalar.
  ExecWait "powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command $\"$$sh=New-Object -ComObject Shell.Application; $$f=$$sh.NameSpace('$SMPROGRAMS'); $$i=$$f.ParseName('${APPNAME}.lnk'); if($$i){$$i.InvokeVerb('taskbarpin')}$\""

  Sleep 800
  IfFileExists "$APPDATA\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar\${APPNAME}.lnk" pin_done
  IfSilent pin_done
  MessageBox MB_ICONINFORMATION|MB_OK "Windows no ha permitido anclar ${APPNAME} automaticamente.$\r$\n$\r$\nPara anclarlo, abre el menu Inicio, haz clic derecho sobre ${APPNAME} y elige Anclar a la barra de tareas."
  pin_done:
SectionEnd

; ── Descripciones de componentes ────────────────────────────────────────────
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_MAIN}      "Instala ${APPNAME}, sus iconos, el motor de WhatsApp y el menu contextual."
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_STARTMENU} "Crea un acceso directo en el menu Inicio."
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_DESKTOP}   "Crea un acceso directo en el escritorio."
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_PIN}       "Intenta anclar ${APPNAME} a la barra de tareas (puede requerir hacerlo a mano segun la version de Windows)."
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; ── Desinstalador ───────────────────────────────────────────────────────────
Section "Uninstall"
  ExecWait '"$INSTDIR\${APPEXE}" --task context-menu-uninstall'

  ; Quitar accesos directos (el anclaje se deshace best-effort, sin abrir la app).
  ExecWait "powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -Command $\"$$sh=New-Object -ComObject Shell.Application; $$f=$$sh.NameSpace('$SMPROGRAMS'); $$i=$$f.ParseName('${APPNAME}.lnk'); if($$i){$$i.InvokeVerb('taskbarunpin')}$\""
  Delete "$SMPROGRAMS\${APPNAME}.lnk"
  Delete "$DESKTOP\${APPNAME}.lnk"

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
  DeleteRegKey HKCU "Software\${REGKEY}"
SectionEnd
