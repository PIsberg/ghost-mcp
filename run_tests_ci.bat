@echo off
set "VCPKG=%USERPROFILE%\vcpkg\installed\x64-mingw-dynamic"
set "PATH=%VCPKG%\bin;C:\ProgramData\mingw64\mingw64\bin;%PATH%"
set "CGO_CPPFLAGS=-I%USERPROFILE%/vcpkg/installed/x64-mingw-dynamic/include"
set "CGO_LDFLAGS=-L%USERPROFILE%/vcpkg/installed/x64-mingw-dynamic/lib"
set "CGO_ENABLED=1"
go test -short -count=1 -v ./internal/ocr/... ./cmd/ghost-mcp/...
