; ============================================================================
;  Compass - Instalador generico para proyectos (NSIS / Modern UI 2)
;
;  Lo genera Compass en tiempo de release: el binario embebe esta plantilla y
;  sustituye los marcadores @...@ por los valores del proyecto antes de invocar
;  makensis. NO se ejecuta a mano.
;
;  Pagina final con dos casillas:
;    - Crear un acceso directo en el escritorio
;    - Ejecutar <App> ahora
; ============================================================================

Unicode true

!include "MUI2.nsh"
!include "LogicLib.nsh"

Name "@APPNAME@"
OutFile "@OUTFILE@"
InstallDir "$LOCALAPPDATA\Programs\@APPNAME@"
InstallDirRegKey HKCU "Software\@REGKEY@" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
BrandingText "@APPNAME@ @APPVER@"

!if "@APPICON@" != ""
  Icon          "@APPICON@"
  UninstallIcon "@APPICON@"
  !define MUI_ICON   "@APPICON@"
  !define MUI_UNICON "@APPICON@"
!endif

!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "Bienvenido a @APPNAME@"
!define MUI_WELCOMEPAGE_TEXT "Este asistente instalara @APPNAME@ en tu equipo.$\r$\n$\r$\nAl terminar podras crear un acceso directo en el escritorio y ejecutar la aplicacion.$\r$\n$\r$\nPulsa Siguiente para continuar."

; Pagina final: casilla de acceso directo + casilla de ejecutar.
!define MUI_FINISHPAGE_RUN "$INSTDIR\@APPEXE@"
!define MUI_FINISHPAGE_RUN_TEXT "Ejecutar @APPNAME@ ahora"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\@APPEXE@"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Crear un acceso directo en el escritorio"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!define MUI_FINISHPAGE_TEXT "@APPNAME@ se ha instalado correctamente.$\r$\n$\r$\nPuedes abrirlo desde el menu Inicio o el escritorio."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Spanish"

Function CreateDesktopShortcut
  CreateShortcut "$DESKTOP\@APPNAME@.lnk" "$INSTDIR\@APPEXE@"
FunctionEnd

Section "Instalar"
  SectionIn RO
  SetOutPath "$INSTDIR"
  File "@APPSRC@"
; Si el proyecto usa el motor de WhatsApp, Compass sustituye este marcador por
; la inclusion de node.exe y del bundle en $INSTDIR\resources.
@WHATSAPP_FILES@

  CreateDirectory "$SMPROGRAMS\@APPNAME@"
  CreateShortcut "$SMPROGRAMS\@APPNAME@\@APPNAME@.lnk" "$INSTDIR\@APPEXE@"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKCU "Software\@REGKEY@" "InstallDir" "$INSTDIR"

  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "DisplayName"     "@APPNAME@"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "DisplayVersion"  "@APPVER@"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "Publisher"       "@APPNAME@"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "DisplayIcon"     "$INSTDIR\@APPEXE@,0"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "InstallLocation" "$INSTDIR"
  WriteRegStr   HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@" "NoRepair" 1
SectionEnd

Section "Uninstall"
  Delete "$DESKTOP\@APPNAME@.lnk"
  Delete "$SMPROGRAMS\@APPNAME@\@APPNAME@.lnk"
  RMDir  "$SMPROGRAMS\@APPNAME@"
  Delete "$INSTDIR\@APPEXE@"
; Companero del bloque @WHATSAPP_FILES@: borra el motor al desinstalar.
@WHATSAPP_UNINSTALL@
  Delete "$INSTDIR\Uninstall.exe"
  RMDir  "$INSTDIR"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@"
  DeleteRegKey HKCU "Software\@REGKEY@"
SectionEnd
