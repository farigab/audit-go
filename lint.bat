@echo off
REM lint.bat - runs golangci-lint on the Audit-GO codebase (Windows)
REM Usage: run from repository root: lint.bat

where golangci-lint >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo [ERRO] golangci-lint nao encontrado no PATH.
    echo Instale em: https://golangci-lint.run/usage/install/
    exit /b 1
)

echo Executando golangci-lint...
golangci-lint run

if %ERRORLEVEL% neq 0 (
    echo.
    echo [FALHA] Lint encontrou problemas. Corrija antes de commitar.
    exit /b 1
)

echo [OK] Sem problemas encontrados.
