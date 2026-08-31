Unicode true

!ifndef APP_VERSION
    !error "APP_VERSION is required"
!endif
!ifndef APP_ARCH
    !error "APP_ARCH is required"
!endif
!ifndef APP_BINARY
    !error "APP_BINARY is required"
!endif
!ifndef WORKER_BINARY
    !error "WORKER_BINARY is required"
!endif
!ifndef ASKPASS_BINARY
    !error "ASKPASS_BINARY is required"
!endif
!ifndef UPDATER_BINARY
    !error "UPDATER_BINARY is required"
!endif
!ifndef OUTPUT_FILE
    !error "OUTPUT_FILE is required"
!endif

!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WinVer.nsh"

!define PRODUCT_NAME "OneCatch"
!define PRODUCT_PUBLISHER "OpenModu"
!define PRODUCT_WEB_SITE "https://github.com/openmodu/onecatch"
!define PRODUCT_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\OneCatch"

Name "${PRODUCT_NAME}"
OutFile "${OUTPUT_FILE}"
InstallDir "$LOCALAPPDATA\Programs\${PRODUCT_NAME}"
InstallDirRegKey HKCU "Software\OpenModu\OneCatch" "InstallDir"
RequestExecutionLevel user
SetCompressor /SOLID lzma
ShowInstDetails show
ShowUninstDetails show
ManifestDPIAware true

VIProductVersion "${APP_VERSION}.0"
VIFileVersion "${APP_VERSION}.0"
VIAddVersionKey "CompanyName" "${PRODUCT_PUBLISHER}"
VIAddVersionKey "FileDescription" "${PRODUCT_NAME} Installer"
VIAddVersionKey "ProductVersion" "${APP_VERSION}"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "LegalCopyright" "(c) 2026, OpenModu"
VIAddVersionKey "ProductName" "${PRODUCT_NAME}"

!define MUI_ABORTWARNING
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\onecatch.exe"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

Function .onInit
    ${IfNot} ${AtLeastWin10}
        MessageBox MB_OK|MB_ICONSTOP "OneCatch requires Windows 10 or later."
        Quit
    ${EndIf}
FunctionEnd

Section "Install"
    SetShellVarContext current

    SetOutPath "$INSTDIR"
    File "/oname=onecatch.exe" "${APP_BINARY}"
    File "/oname=onecatch-worker.exe" "${WORKER_BINARY}"
    File "/oname=onecatch-askpass.exe" "${ASKPASS_BINARY}"
    File "/oname=onecatch-updater.exe" "${UPDATER_BINARY}"

    SetOutPath "$TEMP\OneCatchInstaller"
    File "/oname=MicrosoftEdgeWebview2Setup.exe" "MicrosoftEdgeWebview2Setup.exe"
    SetRegView 64
    ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${If} $0 == ""
        ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
    ${EndIf}
    ${If} $0 == ""
        DetailPrint "Installing Microsoft Edge WebView2 Runtime..."
        ExecWait '"$TEMP\OneCatchInstaller\MicrosoftEdgeWebview2Setup.exe" /silent /install' $1
    ${EndIf}
    Delete "$TEMP\OneCatchInstaller\MicrosoftEdgeWebview2Setup.exe"
    RMDir "$TEMP\OneCatchInstaller"
    SetRegView 32

    SetOutPath "$INSTDIR"
    WriteUninstaller "$INSTDIR\uninstall.exe"
    CreateDirectory "$SMPROGRAMS\${PRODUCT_NAME}"
    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk" "$INSTDIR\onecatch.exe"
    CreateShortcut "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall ${PRODUCT_NAME}.lnk" "$INSTDIR\uninstall.exe"
    CreateShortcut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\onecatch.exe"

    WriteRegStr HKCU "Software\OpenModu\OneCatch" "InstallDir" "$INSTDIR"
    WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "DisplayName" "${PRODUCT_NAME}"
    WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "DisplayVersion" "${APP_VERSION}"
    WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "DisplayIcon" "$INSTDIR\onecatch.exe"
    WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "Publisher" "${PRODUCT_PUBLISHER}"
    WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
    WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
    WriteRegDWORD HKCU "${PRODUCT_UNINST_KEY}" "NoModify" 1
    WriteRegDWORD HKCU "${PRODUCT_UNINST_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
    SetShellVarContext current
    Delete "$DESKTOP\${PRODUCT_NAME}.lnk"
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\${PRODUCT_NAME}.lnk"
    Delete "$SMPROGRAMS\${PRODUCT_NAME}\Uninstall ${PRODUCT_NAME}.lnk"
    RMDir "$SMPROGRAMS\${PRODUCT_NAME}"

    Delete "$INSTDIR\onecatch.exe"
    Delete "$INSTDIR\onecatch-worker.exe"
    Delete "$INSTDIR\onecatch-askpass.exe"
    Delete "$INSTDIR\onecatch-updater.exe"
    Delete "$INSTDIR\uninstall.exe"
    RMDir "$INSTDIR"

    DeleteRegKey HKCU "${PRODUCT_UNINST_KEY}"
    DeleteRegKey HKCU "Software\OpenModu\OneCatch"
SectionEnd
