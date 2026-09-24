#ifndef Version
#define Version "0.4.5"
#endif

[Setup]
AppId={{5E97D5B6-D44E-4623-A7C7-9FCE7E4256E3}
AppName=QuickFlare
AppVersion={#Version}
AppVerName=QuickFlare v{#Version}
AppPublisher=San-Shiro
AppPublisherURL=https://github.com/San-Shiro/QuickFlare
AppSupportURL=https://github.com/San-Shiro/QuickFlare/issues
AppUpdatesURL=https://github.com/San-Shiro/QuickFlare/releases
DefaultDirName={localappdata}\QuickFlare
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputBaseFilename=QuickFlare-{#Version}-Setup
OutputDir=..\build
Compression=lzma2/ultra64
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=..\assets\quickflare.ico
UninstallDisplayIcon={app}\quickflare.ico
WizardStyle=modern
CloseApplications=yes
RestartApplications=no

[Tasks]
Name: "trayapp"; Description: "Install System Tray Application (Recommended)"; GroupDescription: "Experience Selection:"
Name: "trayapp\autostart"; Description: "Start QuickFlare when Windows starts"
Name: "addtopath"; Description: "Add QuickFlare to User PATH (enables 'quickflare' in CMD/PowerShell)"; GroupDescription: "Environment:"; Flags: checkedonce

[Files]
Source: "..\build\quickflare.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\build\quickflare-tray.exe"; DestDir: "{app}"; Flags: ignoreversion; Tasks: trayapp
Source: "..\assets\quickflare.ico"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{userprograms}\QuickFlare"; Filename: "{app}\quickflare-tray.exe"; IconFilename: "{app}\quickflare.ico"; Tasks: trayapp
Name: "{userstartup}\QuickFlare"; Filename: "{app}\quickflare-tray.exe"; Tasks: trayapp\autostart

[Run]
Filename: "{app}\quickflare-tray.exe"; Description: "Launch QuickFlare"; Flags: postinstall nowait skipifsilent; Tasks: trayapp

[Code]
const
  WM_SETTINGCHANGE = $001A;
  SMTO_ABORTIFHUNG = $0002;

function SendMessageTimeout(hWnd: HWND; Msg: UINT; wParam: LongInt; lParam: String; fuFlags: UINT; uTimeout: UINT; out lpdwResult: LongInt): LongInt;
  external 'SendMessageTimeoutW@user32.dll stdcall';

procedure RefreshEnvironment();
var
  res: LongInt;
begin
  SendMessageTimeout(HWND_BROADCAST, WM_SETTINGCHANGE, 0, 'Environment', SMTO_ABORTIFHUNG, 5000, res);
end;

procedure AddAppToPath();
var
  currentPath: string;
  appDir: string;
begin
  appDir := ExpandConstant('{app}');
  if RegQueryStringValue(HKCU, 'Environment', 'Path', currentPath) then
  begin
    if Pos(';' + UpperCase(appDir) + ';', ';' + UpperCase(currentPath) + ';') = 0 then
    begin
      if (Length(currentPath) > 0) and (currentPath[Length(currentPath)] <> ';') then
        currentPath := currentPath + ';';
      currentPath := currentPath + appDir;
      RegWriteStringValue(HKCU, 'Environment', 'Path', currentPath);
      RefreshEnvironment();
    end;
  end
  else
  begin
    RegWriteStringValue(HKCU, 'Environment', 'Path', appDir);
    RefreshEnvironment();
  end;
end;

procedure RemoveAppFromPath();
var
  currentPath: string;
  appDir: string;
  p: Integer;
begin
  appDir := ExpandConstant('{app}');
  if RegQueryStringValue(HKCU, 'Environment', 'Path', currentPath) then
  begin
    p := Pos(UpperCase(appDir) + ';', UpperCase(currentPath));
    if p > 0 then
      Delete(currentPath, p, Length(appDir) + 1)
    else
    begin
      p := Pos(';' + UpperCase(appDir), UpperCase(currentPath));
      if p > 0 then
        Delete(currentPath, p, Length(appDir) + 1)
      else if UpperCase(currentPath) = UpperCase(appDir) then
        currentPath := '';
    end;
    RegWriteStringValue(HKCU, 'Environment', 'Path', currentPath);
    RefreshEnvironment();
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    if WizardIsTaskSelected('addtopath') then
      AddAppToPath();
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  resultCode: Integer;
begin
  if CurUninstallStep = usUninstall then
  begin
    if MsgBox('Do you want to gracefully tear down and unpublish all active Cloudflare routes via API before removing QuickFlare?' + #13#10 + #13#10 + '(Recommended: Yes)', mbConfirmation, MB_YESNO or MB_DEFBUTTON1) = IDYES then
    begin
      Exec(ExpandConstant('{app}\quickflare.exe'), 'uninstall --silent --routes-only', '', SW_HIDE, ewWaitUntilTerminated, resultCode);
    end;
    RemoveAppFromPath();
  end;
end;
