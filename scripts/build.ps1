param([string]$Version='0.1.0', [switch]$RequireSignature)
$ErrorActionPreference='Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$payload=Join-Path $PWD 'dist/payload'
New-Item -ItemType Directory -Force $payload | Out-Null
$env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
& go build -trimpath -ldflags "-s -w -X main.version=$Version" -o "$payload/ezgh-obs-agent-windows.exe" ./cmd/ezgh-obs-agent-windows
if($LASTEXITCODE){throw 'Go build failed'}
Copy-Item LICENSE,NOTICE $payload
Copy-Item examples/config.yaml "$payload/config.example.yaml"
# Signing uses an already-provisioned certificate in the Windows certificate store.
function Sign-Artifact([string]$Path) {
 if($env:EZGH_SIGNING_THUMBPRINT) {
  & signtool sign /sha1 $env:EZGH_SIGNING_THUMBPRINT /fd SHA256 /tr https://timestamp.digicert.com /td SHA256 $Path
  if($LASTEXITCODE){throw 'Signing failed'}
  & signtool verify /pa $Path
  if($LASTEXITCODE){throw 'Signature verification failed'}
 } elseif($RequireSignature){throw 'Production artifacts require EZGH_SIGNING_THUMBPRINT'}
}
# Include license texts for modules linked into the executable.
$notices = [Collections.Generic.List[string]]::new()
$linked = & go list -deps -f '{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}' ./cmd/ezgh-obs-agent-windows
if($LASTEXITCODE){throw 'Dependency license inventory failed'}
foreach($row in ($linked | Where-Object {$_} | Sort-Object -Unique)) {
 $parts=$row -split '\|',3
 if($parts[0] -eq 'github.com/ezgamehost/ezgh-obs-agent-windows'){continue}
 $notices.Add("Dependency: $($parts[0]) $($parts[1])")
 $licenses=@(Get-ChildItem -LiteralPath $parts[2] -File | Where-Object {$_.Name -match '^(LICENSE|COPYING|NOTICE)(\..*)?$'})
 if($licenses.Count -eq 0){throw "Missing dependency license: $($parts[0])"}
 foreach($license in $licenses){$notices.Add([IO.File]::ReadAllText($license.FullName))}
}
$notices | Set-Content -Encoding utf8 "$payload/ThirdPartyNotices.txt"
Sign-Artifact "$payload/ezgh-obs-agent-windows.exe"
$tools=Join-Path $PWD 'dist/tools'
if(-not(Test-Path "$tools/wix.exe")) { & dotnet tool install wix --version 5.0.2 --tool-path $tools; if($LASTEXITCODE){throw 'WiX install failed'} }
$hash=[Security.Cryptography.SHA1]::Create().ComputeHash([Text.Encoding]::Unicode.GetBytes('EZGH-OBS-AGENT-WINDOWS'))
$sid='S-1-5-80-'+((0..4|ForEach-Object{[BitConverter]::ToUInt32($hash,$_ * 4)}) -join '-')
$suffix=if($env:EZGH_SIGNING_THUMBPRINT){''}else{'-unsigned-development'}
$msi="dist/ezgh-obs-agent-windows-$Version-x64$suffix.msi"
& "$tools/wix.exe" build installer/Product.wxs -arch x64 -d "Version=$Version" -d "Payload=$payload" -d "ServiceSID=$sid" -o $msi
if($LASTEXITCODE){throw 'MSI build failed'}
Sign-Artifact $msi
$env:GOOS='windows'
& go list -m -json all | Set-Content -Encoding utf8 "dist/modules-$Version.json"
if($LASTEXITCODE){throw 'Dependency inventory failed'}
# Standard CycloneDX inventory records exact pinned module versions; no secrets.
$modules=& go list -m -f '{{.Path}} {{.Version}}' all
$components=@($modules|Select-Object -Skip 1|ForEach-Object{$v=$_ -split ' ';@{type='library';name=$v[0];version=$v[1];purl="pkg:golang/$($v[0])@$($v[1])"}})
@{bomFormat='CycloneDX';specVersion='1.5';version=1;components=$components}|ConvertTo-Json -Depth 8|Set-Content -Encoding utf8 "dist/sbom-$Version.json"
Copy-Item "dist/sbom-$Version.json" "$payload/sbom.json"
Compress-Archive -Path "$payload/*" -DestinationPath "dist/ezgh-obs-agent-windows-$Version-x64$suffix.zip" -Force
Get-ChildItem dist -File | Where-Object {$_.Extension -in '.msi','.zip','.json'} | ForEach-Object { $h=Get-FileHash $_.FullName -Algorithm SHA256; "$($h.Hash.ToLower())  $($_.Name)" } | Set-Content -Encoding ascii dist/SHA256SUMS
