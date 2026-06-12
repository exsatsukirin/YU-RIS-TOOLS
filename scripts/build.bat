@echo off
REM Build yrt for Windows and Linux
setlocal

cd /d "%~dp0\.."

if not exist build mkdir build

echo === Building yrt ===

echo   Windows x86_64...
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -o build\yrt.exe -ldflags="-s -w" .

echo   Linux x86_64...
set GOOS=linux
set GOARCH=amd64
set CGO_ENABLED=0
go build -o build\yrt -ldflags="-s -w" .

echo.
echo Done:
dir build\yrt* 2>nul
