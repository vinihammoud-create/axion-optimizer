; Instalador do Axion Optimizer (Inno Setup 6).
; Compilar: .\build\installer.ps1 -Version 0.9.1  (precisa do pacote de build\build.ps1)
; ou: ISCC.exe /DAppVersion=0.9.1 installer\axion.iss

#ifndef AppVersion
  #error Informe a versão: ISCC.exe /DAppVersion=0.9.1 installer\axion.iss
#endif

#define AppName "Axion Optimizer"
#define PackageDir "..\dist\Axion-" + AppVersion + "-win64"

[Setup]
AppId={{4541B26D-04F0-449D-8AB2-ACB1C82C7AAD}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=Axion
DefaultDirName={autopf}\{#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=..\build\axion.ico
UninstallDisplayIcon={app}\Axion.exe
UninstallDisplayName={#AppName}
VersionInfoVersion={#AppVersion}
OutputDir=..\dist
OutputBaseFilename=Axion-{#AppVersion}-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern

[Languages]
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "{#PackageDir}\Axion.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\WebView2Loader.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\PresentMon-2.6.0-x64.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\LEIA-ME.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\THIRD-PARTY-NOTICES.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\MATRIZ-DE-EVIDENCIAS.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\SHA256SUMS.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\Axion.exe"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\Axion.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\Axion.exe"; Description: "Abrir o Axion Optimizer"; Flags: nowait postinstall skipifsilent

[Code]
const
  WebView2Key = 'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';

function WebView2VersionOk(RootKey: Integer; const SubKey: string): Boolean;
var
  Version: string;
begin
  Result := RegQueryStringValue(RootKey, SubKey, 'pv', Version)
    and (Version <> '') and (Version <> '0.0.0.0');
end;

function WebView2Installed: Boolean;
begin
  Result := WebView2VersionOk(HKLM32, WebView2Key)
    or WebView2VersionOk(HKLM64, WebView2Key)
    or WebView2VersionOk(HKCU, WebView2Key);
end;

procedure CurPageChanged(CurPageID: Integer);
begin
  if (CurPageID = wpFinished) and not WebView2Installed then
    WizardForm.FinishedLabel.Caption := WizardForm.FinishedLabel.Caption + #13#10#13#10 +
      'Aviso: o WebView2 Runtime não foi encontrado neste computador. ' +
      'Sem ele, o Axion abre a interface no navegador padrão.';
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  DataDir: string;
begin
  if CurUninstallStep <> usPostUninstall then
    Exit;
  DataDir := ExpandConstant('{commonappdata}\Axion');
  if not DirExists(DataDir) then
    Exit;
  if SuppressibleMsgBox(
       'Remover também os dados do Axion em ' + DataDir + '?' + #13#10#13#10 +
       'Essa pasta guarda o histórico usado para desfazer os ajustes. ' +
       'Se você a remover, não será mais possível desfazer ajustes já aplicados.',
       mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDNO) = IDYES then
    DelTree(DataDir, True, True, True);
end;
