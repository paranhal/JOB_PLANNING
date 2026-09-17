# JOB_PLANNING 빌드 스크립트  (§40.3 ③)
# 버전·커밋·빌드시각을 실행 파일에 새기고 server-app.tar 를 만든다.
#
#   사용법:  PowerShell 에서  .\deploy\build.ps1
#            또는 deploy\build.bat 을 더블클릭
#
# 이 스크립트를 거치지 않고 docker build 를 직접 치면
# 화면 버전이 dev 로 뜬다. 반드시 이걸로 빌드한다.

$ErrorActionPreference = 'Stop'

# 프로젝트 루트 = 이 스크립트의 부모 폴더
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root

# ── 1. 값 세 개를 만든다 ─────────────────────────────────────────
$VersionFile = Join-Path $Root 'VERSION'
if (Test-Path $VersionFile) {
    $Version = (Get-Content $VersionFile -Raw).Trim()
} else {
    $Version = 'dev'
    Write-Host "  ! VERSION 파일이 없습니다. dev 로 빌드합니다." -ForegroundColor Yellow
}
if ([string]::IsNullOrWhiteSpace($Version)) { $Version = 'dev' }

try {
    $Commit = (git rev-parse --short HEAD 2>$null).Trim()
    if ([string]::IsNullOrWhiteSpace($Commit)) { $Commit = 'nogit' }
} catch {
    $Commit = 'nogit'
}

$BuildTime = Get-Date -Format 'yyyy-MM-dd HH:mm'

Write-Host ""
Write-Host "=== 빌드 정보 =========================================" -ForegroundColor Cyan
Write-Host "  버전      $Version"
Write-Host "  커밋      $Commit"
Write-Host "  빌드시각  $BuildTime"
Write-Host "=======================================================" -ForegroundColor Cyan
Write-Host ""

# ── 2. 도커 이미지를 만든다 ──────────────────────────────────────
docker build `
    --build-arg "VERSION=$Version" `
    --build-arg "COMMIT=$Commit" `
    --build-arg "BUILD_TIME=$BuildTime" `
    -t server-app:latest `
    (Join-Path $Root 'server')
if ($LASTEXITCODE -ne 0) { throw "docker build 실패" }

# ── 3. 이전 tar 를 백업하고 새로 만든다 ──────────────────────────
$Tar     = Join-Path $Root 'deploy\server-app.tar'
$TarPrev = "$Tar.prev"
if (Test-Path $Tar) {
    Copy-Item $Tar $TarPrev -Force
    Write-Host "  이전 이미지를 server-app.tar.prev 로 백업했습니다."
}

docker save server-app:latest -o $Tar
if ($LASTEXITCODE -ne 0) { throw "docker save 실패" }

$SizeMB = [math]::Round((Get-Item $Tar).Length / 1MB, 1)

Write-Host ""
Write-Host "=== 완료 ==============================================" -ForegroundColor Green
Write-Host "  deploy\server-app.tar   ($SizeMB MB)"
Write-Host ""
Write-Host "  서버에서 다음을 실행하세요:" -ForegroundColor Yellow
Write-Host "    docker compose down"
Write-Host "    docker load -i server-app.tar"
Write-Host "    docker compose up -d"
Write-Host "    curl -s localhost:8888/version"
Write-Host ""
Write-Host "  마지막 줄에서 이렇게 나와야 성공입니다:" -ForegroundColor Yellow
Write-Host "    version: $Version   built: $BuildTime"
Write-Host "=======================================================" -ForegroundColor Green
