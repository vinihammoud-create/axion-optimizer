; Instalador do Axion Optimizer (Inno Setup 6, testado com a 6.7.1).
;
; Compilar pelo script, que confere o pacote e passa os parâmetros:
;   .\build\build.ps1 -Version 0.10.0
;   .\build\installer.ps1 -Version 0.10.0
; ou direto (PresentMonFile é o nome do PresentMon dentro do pacote):
;   ISCC.exe /DAppVersion=0.10.0 /DPresentMonFile=PresentMon-2.6.0-x64.exe installer\axion.iss
;
; Assinatura (desligada até haver certificado): com /DSignToolName=<nome> e
; /S<nome>=<comando com $f>, o ISCC assina o instalador e o desinstalador.
; O build\installer.ps1 faz isso com -SignTool '<comando com $f>'.

#ifndef AppVersion
  #error Informe a versão: ISCC.exe /DAppVersion=x.y.z /DPresentMonFile=<arquivo> installer\axion.iss
#endif
#ifndef PresentMonFile
  #error Informe o PresentMon do pacote: /DPresentMonFile=PresentMon-<versão>-x64.exe (o build\installer.ps1 lê do pacote)
#endif
#ifndef PackageDir
  #define PackageDir "..\dist\Axion-" + AppVersion + "-win64"
#endif

#define AppName "Axion Optimizer"
; A mesma trava de cópia única do Axion.exe (cmd\axion\main_windows.go, mutexName).
#define AppMutexName "Global\AxionOptimizer.SingleInstance"

[Setup]
AppId={{4541B26D-04F0-449D-8AB2-ACB1C82C7AAD}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher=Axion
AppPublisherURL=https://github.com/vinihammoud-create/axion-optimizer
AppSupportURL=https://github.com/vinihammoud-create/axion-optimizer/issues
AppUpdatesURL=https://github.com/vinihammoud-create/axion-optimizer/releases
; Não instala nem desinstala com o Axion aberto (em qualquer conta do PC):
; ele pode estar aplicando ou desfazendo um ajuste.
AppMutex={#AppMutexName}
; O Gerenciador de Reinicialização do Windows não fecha o Axion no meio de
; uma tarefa: quem fecha é o usuário (a trava acima espera por isso).
CloseApplications=no
RestartApplications=no
DefaultDirName={autopf}\{#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
LicenseFile={#PackageDir}\LICENSE.txt
SetupIconFile=..\build\axion.ico
UninstallDisplayIcon={app}\Axion.exe
UninstallDisplayName={#AppName}
VersionInfoVersion={#AppVersion}
VersionInfoProductName={#AppName}
VersionInfoProductVersion={#AppVersion}
VersionInfoDescription=Instalador do {#AppName}
OutputDir=..\dist
OutputBaseFilename=Axion-{#AppVersion}-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ShowLanguageDialog=no
SetupLogging=yes
UninstallLogging=yes
#ifdef SignToolName
SignTool={#SignToolName}
SignedUninstaller=yes
#endif

[Languages]
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"

[Messages]
SetupAppRunningError=O %1 está aberto neste PC (talvez em outra conta, ou terminando uma tarefa que estava em andamento).%n%nFeche o Axion e espere ele terminar; depois clique em OK para continuar, ou em Cancelar para sair sem instalar.
UninstallAppRunningError=O %1 está aberto neste PC (talvez em outra conta, ou terminando uma tarefa que estava em andamento).%n%nFeche o Axion e espere ele terminar; depois clique em OK para continuar, ou em Cancelar para sair sem desinstalar.

[CustomMessages]
AboutCaption=O que o Axion faz
AboutDescription=Leia antes de instalar.
AboutLead=O Axion Optimizer é um otimizador para Windows 10 e 11 focado em jogos.
AboutText=• Diagnostica o PC: lê as configurações do Windows e do hardware e mostra o que encontrou. A análise não altera nada.%n%n• Antes de aplicar, mostra cada mudança com a fonte e o risco. Nada é alterado sem você pedir.%n%n• Guarda o valor anterior de cada mudança, para você desfazer depois na página Histórico. O histórico fica em C:\ProgramData\Axion.%n%n• Não instala driver, não injeta nada em jogos e não tem telemetria. Usa a internet só quando você pede: testes de rede, procura de driver de vídeo e instalação de programas pelo winget.%n%n• Pede permissão de administrador sempre que abre, porque as otimizações alteram configurações do Windows.%n%nDesinstalar remove o programa, mas não desfaz as mudanças que você aplicou. Para voltar tudo ao que era, abra o Axion e use Histórico > Desfazer tudo antes de desinstalar.
WebView2Caption=WebView2 Runtime não encontrado
WebView2Description=O Axion funciona, mas vai abrir no navegador.
WebView2Lead=O Axion foi instalado.
WebView2Text=O instalador não encontrou o Microsoft Edge WebView2 Runtime neste Windows. É o componente gratuito da Microsoft que abre a janela própria do Axion.%n%nSem ele, o Axion funciona, mas abre a interface no seu navegador padrão. Nesse modo a chave de acesso à interface passa pelo navegador padrão (e pelas extensões dele) e pela linha de comando que o abre. Na janela própria ela fica só dentro da janela: é o modo recomendado.%n%nRecomendado: instale o WebView2 Runtime pela página oficial da Microsoft:%nhttps://developer.microsoft.com/microsoft-edge/webview2/%n%nO instalador do Axion não baixa nada. Depois de instalar o WebView2, abra o Axion de novo.
WebView2Button=Abrir a página do WebView2
WebView2OpenFailed=Não foi possível abrir o navegador. Abra este endereço:%n%n%1
ActiveWarning=O histórico do Axion registra %1 neste PC: otimizações que continuam aplicadas no Windows.%n%nDesinstalar NÃO desfaz essas alterações. Elas continuam no Windows depois que o programa sair.%n%nRecomendado: clique em Não, abra o Axion e use Histórico > Desfazer tudo. Depois desinstale.%n%nDesinstalar mesmo assim?
ActiveUnknown=O desinstalador não conseguiu ler o histórico do Axion (%1) e por isso não sabe se ainda há alterações do Axion aplicadas neste PC.%n%nDesinstalar não desfaz alterações. Se você aplicou otimizações, clique em Não, abra o Axion e confira a página Histórico antes de desinstalar.%n%nDesinstalar mesmo assim?
RemoveData=Apagar também a pasta de dados do Axion?%n%n%1%n%nEla guarda o histórico com os valores anteriores de cada alteração (usado para desfazer), os arquivos que o Axion guardou para poder restaurar (quarentena), as medições e os registros do programa. Se você instalar o Axion de novo, ele usa essa pasta e continua podendo desfazer o que foi aplicado.%n%nNa dúvida, clique em Não.
RemoveDataActive=Atenção: ainda há alterações do Axion aplicadas neste PC, ou o histórico não pôde ser lido. Apagar esta pasta perde os valores anteriores, e o Axion não consegue mais desfazer essas alterações.

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[InstallDelete]
; Numa atualização, o PresentMon de uma versão anterior sai antes de o novo entrar.
Type: files; Name: "{app}\PresentMon-*.exe"

[Files]
Source: "{#PackageDir}\Axion.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\WebView2Loader.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\{#PresentMonFile}"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\LEIA-ME.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\LICENSE.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\THIRD-PARTY-NOTICES.txt"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\MATRIZ-DE-EVIDENCIAS.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#PackageDir}\SHA256SUMS.txt"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#AppName}"; Filename: "{app}\Axion.exe"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\Axion.exe"; Tasks: desktopicon

[Run]
; O Axion exige administrador (manifesto): roda com os direitos que o
; instalador já tem, sem um segundo pedido.
Filename: "{app}\Axion.exe"; Description: "Abrir o {#AppName}"; Flags: nowait postinstall skipifsilent runascurrentuser

[Code]
const
  WebView2Guid = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
  WebView2Url = 'https://developer.microsoft.com/microsoft-edge/webview2/';
  { Modo do Axion.exe que só lê o histórico e responde pelo código de saída
    (cmd\axion\activechanges.go): 0 nada aplicado, 1 histórico ilegível,
    10+n n alterações ativas (n até 200). }
  ActiveChangesFlag = '--axion-ajustes-ativos';
  ActiveUnknown = -1;

var
  WebView2Present: Boolean;
  WebView2Page: TOutputMsgMemoWizardPage;
  ActiveChanges: Integer;

{ Texto de [CustomMessages] com as quebras de linha prontas. }
function TextOf(const Name: String): String;
begin
  Result := CustomMessage(Name);
  StringChangeEx(Result, '%n', #13#10, True);
end;

function TextWith(const Name, Arg: String): String;
begin
  Result := FmtMessage(TextOf(Name), [Arg]);
end;

{ --- Instalação ----------------------------------------------------------- }

function WebView2VersionOk(RootKey: Integer; const SubKey: String): Boolean;
var
  Version: String;
begin
  Result := RegQueryStringValue(RootKey, SubKey, 'pv', Version)
    and (Version <> '') and (Version <> '0.0.0.0');
end;

{ As chaves que a Microsoft indica para saber se o WebView2 Runtime está
  instalado: para todos os usuários (visão de 32 e de 64 bits) ou só para esta
  conta. O Axion roda como administrador, com a mesma conta deste instalador. }
function WebView2Installed: Boolean;
begin
  Result := WebView2VersionOk(HKLM32, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\' + WebView2Guid)
    or WebView2VersionOk(HKLM64, 'SOFTWARE\Microsoft\EdgeUpdate\Clients\' + WebView2Guid)
    or WebView2VersionOk(HKCU, 'Software\Microsoft\EdgeUpdate\Clients\' + WebView2Guid);
end;

procedure OpenWebView2Url(Sender: TObject);
var
  Code: Integer;
begin
  { O navegador abre como o usuário, nunca como administrador. }
  if not ShellExecAsOriginalUser('open', WebView2Url, '', '', SW_SHOWNORMAL, ewNoWait, Code) then
    MsgBox(TextWith('WebView2OpenFailed', WebView2Url), mbError, MB_OK);
end;

procedure InitializeWizard;
var
  AboutPage: TOutputMsgMemoWizardPage;
  Button: TNewButton;
begin
  WebView2Present := WebView2Installed;

  { Primeira página (a de boas-vindas não aparece no estilo moderno). }
  AboutPage := CreateOutputMsgMemoPage(wpWelcome, TextOf('AboutCaption'), TextOf('AboutDescription'), TextOf('AboutLead'), '');
  AboutPage.RichEditViewer.Lines.Text := TextOf('AboutText');

  { Depois de instalar, só quando o WebView2 Runtime não foi encontrado. Página
    própria em vez de alongar o texto da página final: o texto rola dentro da
    caixa em vez de empurrar a caixa "Abrir o Axion" em escala de 125-150%. }
  WebView2Page := CreateOutputMsgMemoPage(wpInfoAfter, TextOf('WebView2Caption'), TextOf('WebView2Description'), TextOf('WebView2Lead'), '');
  WebView2Page.RichEditViewer.Lines.Text := TextOf('WebView2Text');
  Button := TNewButton.Create(WebView2Page);
  Button.Parent := WebView2Page.Surface;
  Button.Caption := TextOf('WebView2Button');
  Button.Height := WizardForm.NextButton.Height;
  Button.Width := ScaleX(260);
  Button.Left := 0;
  WebView2Page.RichEditViewer.Height := WebView2Page.SurfaceHeight - WebView2Page.RichEditViewer.Top - Button.Height - ScaleY(10);
  Button.Top := WebView2Page.RichEditViewer.Top + WebView2Page.RichEditViewer.Height + ScaleY(10);
  Button.OnClick := @OpenWebView2Url;
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  Result := (WebView2Page <> nil) and (PageID = WebView2Page.ID) and WebView2Present;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
    if WebView2Present then
      Log('WebView2 Runtime encontrado.')
    else
      Log('WebView2 Runtime não encontrado: o Axion vai abrir no navegador até ele ser instalado.');
end;

{ --- Desinstalação -------------------------------------------------------- }

function AxionDataDir: String;
begin
  Result := ExpandConstant('{commonappdata}\Axion');
end;

{ Pergunta ao próprio Axion.exe (com a regra de "ativa" do programa) quantas
  alterações do histórico ainda estão aplicadas. Qualquer falha é "não sei",
  nunca "nada aplicado". }
function ReadActiveChanges: Integer;
var
  Exe: String;
  Code: Integer;
begin
  Result := ActiveUnknown;
  Exe := ExpandConstant('{app}\Axion.exe');
  if not FileExists(Exe) then begin
    Log('Axion.exe não encontrado: o histórico não foi verificado.');
    Exit;
  end;
  if not Exec(Exe, ActiveChangesFlag, ExpandConstant('{app}'), SW_HIDE, ewWaitUntilTerminated, Code) then begin
    Log('Não foi possível executar o Axion.exe para ler o histórico: ' + SysErrorMessage(Code));
    Exit;
  end;
  Log(Format('Axion.exe %s terminou com o código %d.', [ActiveChangesFlag, Code]));
  if Code = 0 then
    Result := 0
  else if (Code > 10) and (Code <= 210) then
    Result := Code - 10;
end;

function ActiveText(N: Integer): String;
begin
  if N = 1 then
    Result := '1 alteração ativa'
  else if N >= 200 then
    Result := '200 ou mais alterações ativas'
  else
    Result := IntToStr(N) + ' alterações ativas';
end;

{ Antes de remover qualquer arquivo (o Axion já está fechado: a trava passou).
  Em modo silencioso a desinstalação segue, e o aviso fica no log. }
procedure WarnActiveChanges;
var
  Question: String;
begin
  ActiveChanges := ReadActiveChanges;
  if ActiveChanges = 0 then begin
    Log('Histórico lido: nenhuma alteração do Axion ativa.');
    Exit;
  end;
  if ActiveChanges > 0 then begin
    Log('Histórico lido: ' + ActiveText(ActiveChanges) + '.');
    Question := TextWith('ActiveWarning', ActiveText(ActiveChanges));
  end else begin
    Log('Histórico não lido: não se sabe se há alterações do Axion ativas.');
    Question := TextWith('ActiveUnknown', AxionDataDir + '\journal.json');
  end;
  if SuppressibleMsgBox(Question, mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDYES) <> IDYES then begin
    Log('Desinstalação cancelada pelo usuário para desfazer as alterações antes.');
    Abort;
  end;
end;

{ Cache, não dado: o perfil do WebView2 em %LOCALAPPDATA% (que o navegador
  recria), a cópia do WebView2Loader.dll que o Axion faz para a pasta
  protegida e o perfil que rascunhos da 0.10.0 tentaram criar na pasta de
  dados. DelTree apaga uma junção sem entrar nela. }
procedure RemoveCaches;
var
  OldDir: String;
begin
  if DirExists(AxionDataDir + '\WebView2') then
    if DelTree(AxionDataDir + '\WebView2', True, True, True) then
      Log('Perfil do WebView2 removido: ' + AxionDataDir + '\WebView2')
    else
      Log('Perfil do WebView2 não removido por inteiro: ' + AxionDataDir + '\WebView2');
  if FileExists(AxionDataDir + '\bin\WebView2Loader.dll') then
    DeleteFile(AxionDataDir + '\bin\WebView2Loader.dll');
  RemoveDir(AxionDataDir + '\bin');
  OldDir := ExpandConstant('{localappdata}\Axion');
  if DirExists(OldDir + '\WebView2') then
    if DelTree(OldDir + '\WebView2', True, True, True) then
      Log('Perfil antigo do WebView2 removido: ' + OldDir + '\WebView2')
    else
      Log('Perfil antigo do WebView2 não removido por inteiro: ' + OldDir + '\WebView2');
  RemoveDir(OldDir);
end;

{ A pasta de dados só sai com um "Sim" explícito; o padrão (e o modo
  silencioso) é manter. }
procedure AskRemoveData;
var
  Question: String;
begin
  if not DirExists(AxionDataDir) then
    Exit;
  Question := TextWith('RemoveData', AxionDataDir);
  if ActiveChanges <> 0 then
    Question := Question + #13#10#13#10 + TextOf('RemoveDataActive');
  if SuppressibleMsgBox(Question, mbConfirmation, MB_YESNO or MB_DEFBUTTON2, IDNO) = IDYES then begin
    if DelTree(AxionDataDir, True, True, True) then
      Log('Pasta de dados removida a pedido do usuário: ' + AxionDataDir)
    else
      Log('Pasta de dados não removida por inteiro: ' + AxionDataDir);
  end else
    Log('Pasta de dados mantida: ' + AxionDataDir);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  case CurUninstallStep of
    usUninstall:
      WarnActiveChanges;
    usPostUninstall:
      begin
        RemoveCaches;
        AskRemoveData;
      end;
  end;
end;
