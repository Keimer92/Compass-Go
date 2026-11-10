@echo off
REM Construye compass-go.exe con icono + version + manifest via go-winres.
REM
REM   go install github.com/tc-hib/go-winres@latest
REM
REM El icono (assets/icon/Icon.ico) y los metadatos viven en go-winres.json;
REM los .syso se generan en cmd\compass\ y los recoge `go build`.
REM
REM Si go-winres no esta instalado, construye igualmente sin recursos de Windows.

where go-winres >nul 2>nul
if %ERRORLEVEL%==0 (
    go-winres make --in go-winres.json --out cmd\compass\rsrc --arch amd64,386
) else (
    echo [aviso] go-winres no encontrado: se construye sin icono/version embebidos.
    echo         Instalalo con: go install github.com/tc-hib/go-winres@latest
)

go build -o compass-go.exe ./cmd/compass
if %ERRORLEVEL%==0 (
    echo [ok] compass-go.exe generado.
) else (
    echo [error] fallo la compilacion.
    exit /b 1
)
