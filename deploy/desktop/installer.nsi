; Windows installer for the LeftOff desktop app (Phase 6).
; Built by deploy/desktop/build.sh: makensis -DVERSION=.. -DSRC=<dir> -DOUT=<setup.exe>
; Installs for the current user (no admin rights), with Start menu and
; desktop shortcuts and an uninstaller. The library stays in
; %APPDATA%\leftoff and is never removed by the uninstaller.
Unicode true
!include "MUI2.nsh"

Name "LeftOff"
OutFile "${OUT}"
InstallDir "$LOCALAPPDATA\Programs\LeftOff"
RequestExecutionLevel user
SetCompressor /SOLID lzma

!define MUI_ICON "${SRC}\icon.ico"
!define MUI_UNICON "${SRC}\icon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\leftoff.exe"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "PortugueseBR"
!insertmacro MUI_LANGUAGE "English"

!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\LeftOff"

Section "LeftOff"
  SetOutPath "$INSTDIR"
  File "${SRC}\leftoff.exe"
  File "${SRC}\icon.ico"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateDirectory "$SMPROGRAMS\LeftOff"
  CreateShortcut "$SMPROGRAMS\LeftOff\LeftOff.lnk" "$INSTDIR\leftoff.exe" "" "$INSTDIR\icon.ico"
  CreateShortcut "$DESKTOP\LeftOff.lnk" "$INSTDIR\leftoff.exe" "" "$INSTDIR\icon.ico"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "LeftOff"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\icon.ico"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\leftoff.exe"
  Delete "$INSTDIR\icon.ico"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\LeftOff\LeftOff.lnk"
  RMDir "$SMPROGRAMS\LeftOff"
  Delete "$DESKTOP\LeftOff.lnk"
  DeleteRegKey HKCU "${UNINST_KEY}"
SectionEnd
