; ============================================================================
;  Compass - Instalador de CARPETA para proyectos (NSIS / Modern UI 2)
;
;  Variante de app-installer.nsi para releases que son una carpeta completa
;  (p. ej. Flutter: exe + DLLs + data/ + whatsapp_engine/). En vez de un solo
;  File usa File /r con todo el contenido del release.
;
;  Lo genera Compass en tiempo de release: el binario embebe esta plantilla y
;  sustituye los marcadores @...@ antes de invocar makensis. NO se ejecuta a mano.
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
  ; Copia el release completo conservando subcarpetas (data/, whatsapp_engine/).
  File /r "@PAYLOADDIR@\*.*"

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
  Delete "$INSTDIR\Uninstall.exe"
  ; El release es una carpeta entera: se borra completa.
  RMDir /r "$INSTDIR"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\@REGKEY@"
  DeleteRegKey HKCU "Software\@REGKEY@"
SectionEnd
