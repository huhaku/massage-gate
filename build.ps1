# massage-gate 一键构建脚本(Windows)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

Write-Host "[1/3] 构建前端..." -ForegroundColor Cyan
Push-Location web-src
npm install --no-audit --no-fund
npm run build
Pop-Location

Write-Host "[2/3] 同步前端产物到嵌入目录..." -ForegroundColor Cyan
if (Test-Path internal/web/dist) { Remove-Item -Recurse -Force internal/web/dist }
Copy-Item -Recurse web-src/dist internal/web/dist

Write-Host "[3/3] 编译 Go 二进制..." -ForegroundColor Cyan
go build -o massage-gate.exe ./cmd/server
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Write-Host "完成: massage-gate.exe" -ForegroundColor Green
