@echo off

where make >nul 2>nul
IF %ERRORLEVEL% EQU 0 (
    make check
) ELSE (
    echo make nao encontrado, rodando direto...

    go fmt ./...
    IF %ERRORLEVEL% NEQ 0 exit /b 1

    go vet ./...
    IF %ERRORLEVEL% NEQ 0 exit /b 1

    go test ./...
    IF %ERRORLEVEL% NEQ 0 exit /b 1
)

exit /b %ERRORLEVEL%
