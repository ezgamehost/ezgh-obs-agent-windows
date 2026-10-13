$ErrorActionPreference='Stop'
if($env:GITHUB_ACTIONS -ne 'true'){throw 'This destructive install test runs only on disposable GitHub Actions runners.'}
Set-Location (Split-Path $PSScriptRoot -Parent)
$service='ezgh-obs-agent-windows'
$base=Join-Path ([Environment]::GetFolderPath('CommonApplicationData')) 'EZGH/ObsAgent'
$exe=Join-Path $env:ProgramFiles 'EZGH/ObsAgent/ezgh-obs-agent-windows.exe'
function MSI([string]$ArgsText,[string]$Log){$p=Start-Process msiexec.exe -ArgumentList "$ArgsText /qn /norestart /l*v `"$PWD/dist/$Log.log`"" -Wait -PassThru;if($p.ExitCode -notin @(0,3010)){throw "MSI failed: $($p.ExitCode); see $Log.log"}}
$msi=(Resolve-Path 'dist/*0.1.0*x64*.msi').Path
MSI "/i `"$msi`"" 'install'
if((Get-Service $service).Status -ne 'Stopped'){throw 'Fresh installation started automatically'}
$config=(Get-Content examples/config.yaml -Raw).Replace('https://obs-ingest.ezghcloud.com','https://127.0.0.1:1').Replace('interval: 30s','interval: 1s')
[IO.File]::WriteAllText("$base/config/config.yaml",$config)
[IO.File]::WriteAllText("$base/secrets/ingest-key",('ezgh_ik_'+('a'*43)))
& $exe repair-permissions
if($LASTEXITCODE){throw 'Permissions failed'}
& $exe activate
if($LASTEXITCODE){throw 'Activation failed'}
$ready=$false
for($attempt=0;$attempt -lt 30;$attempt++) {
 Start-Sleep 1
 $raw=& $exe status
 if($LASTEXITCODE -eq 0){$snapshot=$raw | ConvertFrom-Json;if($snapshot.status.states.metrics_collection -eq 'ready'){$ready=$true;break}}
}
if(-not $ready){throw 'Default collectors failed under the installed virtual service account'}
& $exe status
if($LASTEXITCODE){throw 'Status failed'}
& $exe doctor
if($LASTEXITCODE){throw 'Doctor failed'}
[IO.File]::WriteAllText("$base/state/preservation-fixture",'keep')
$idHash=(Get-FileHash "$base/state/identity.db").Hash
./scripts/build.ps1 -Version 0.1.1
$new=(Resolve-Path 'dist/*0.1.1*x64*.msi').Path
MSI "/i `"$new`"" 'upgrade'
Start-Sleep 3
if((Get-Service $service).Status -ne 'Running'){throw 'Upgrade failed to restore running state'}
if(-not(Test-Path "$base/state/preservation-fixture")){throw 'Upgrade removed state'}
& $exe status
if($LASTEXITCODE){throw 'Upgraded status failed'}
MSI "/fa `"$new`"" 'repair'
Start-Sleep 3
if((Get-Service $service).Status -ne 'Running'){throw 'Repair failed to restore running state'}
MSI "/x `"$new`"" 'uninstall'
if(Get-Service $service -ErrorAction SilentlyContinue){throw 'Uninstall left service'}
foreach($p in @('state/identity.db','state/preservation-fixture','config/config.yaml','secrets/ingest-key')){if(-not(Test-Path "$base/$p")){throw "Uninstall removed $p"}}
