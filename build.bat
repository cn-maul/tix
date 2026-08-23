@echo off
chcp 65001 >nul
cd /d "%~dp0"

echo ============================================
echo   tix ticket system - build (frontend + Go)
echo ============================================
echo.

echo [1/2] building frontend ...
if not exist "web\node_modules\vite" (
    echo     installing deps ...
    call pnpm --dir web install --ignore-scripts --force
    if errorlevel 1 (
        echo [FAIL] frontend deps install error
        pause
        exit /b 1
    )
) else (
    echo     deps present, skipping install
)
call pnpm --dir web build
if errorlevel 1 (
    echo.
    echo [FAIL] frontend build error
    pause
    exit /b 1
)

echo.
echo [2/2] building Go ...
go build -o tix.exe .
if errorlevel 1 (
    echo.
    echo [FAIL] Go build error - install Go 1.25+ first
    pause
    exit /b 1
)

echo.
echo [OK] tix.exe built (single file, frontend embedded)
pause