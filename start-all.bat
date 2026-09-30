@echo off
setlocal
title ServerHub - Dev Launcher

REM Always operate from the repo root (folder this file lives in).
cd /d "%~dp0"
echo ============================================
echo  ServerHub dev launcher
echo  Root: %CD%
echo ============================================
echo.

REM --- Prerequisite checks (fail fast with a clear message) ---
where go >nul 2>nul
if %errorlevel% neq 0 (
  echo ERROR: 'go' not found on PATH. Install Go ^>= 1.26 first.
  pause
  exit /b 1
)
where npm >nul 2>nul
if %errorlevel% neq 0 (
  echo ERROR: 'npm' not found on PATH. Install Node 22 LTS first.
  pause
  exit /b 1
)
if not exist "%CD%\server\go.mod" (
  echo ERROR: server\go.mod not found. This file must live in the repo root.
  pause
  exit /b 1
)
if not exist "%CD%\server\.air.toml" (
  echo ERROR: server\.air.toml not found. This file must live in the repo root.
  pause
  exit /b 1
)
if not exist "%CD%\client\package.json" (
  echo ERROR: client\package.json not found. This file must live in the repo root.
  pause
  exit /b 1
)

REM --- Air (Go live reload for the backend) ---
where air >nul 2>nul
if %errorlevel% neq 0 (
  echo air not found - installing github.com/air-verse/air@latest ...
  go install github.com/air-verse/air@latest
  set "PATH=%PATH%;%USERPROFILE%\go\bin"
  where air >nul 2>nul
  if %errorlevel% neq 0 (
    echo ERROR: 'air' still not available. Add %USERPROFILE%\go\bin to PATH and re-run.
    pause
    exit /b 1
  )
)
echo air: live reload enabled for the backend.
echo.

REM --- 1. VS Code ---
where code >nul 2>nul
if %errorlevel%==0 (
  echo [1/4] Opening VS Code...
  start "" code "%CD%"
) else (
  echo [1/4] WARNING: 'code' not on PATH, skipping VS Code launch.
  echo        Open the folder manually, or enable the 'code' shell command.
)
echo.

REM --- 2. Backend (air, live reload, API on :4000) ---
echo [2/4] Starting backend with air in a new window...
echo        Postgres must be running on localhost:5432.
start "ServerHub - Backend (air :4000)" /d "%CD%\server" cmd /k "echo [backend] air - live reload, rebuilds on .go changes && air"
echo.

REM --- 3. Frontend (Vite dev server on :6540) ---
echo [3/4] Starting frontend in a new window...
echo        First boot takes ~10-20s while Vite optimizes dependencies.
start "ServerHub - Frontend (:6540)" /d "%CD%\client" cmd /k "if not exist node_modules (echo [frontend] First run - installing dependencies... && npm install) && npm run dev"
echo.

REM --- 4. Wait until both answer, then open browsers ---
where curl >nul 2>nul
if %errorlevel% neq 0 (
  echo [4/4] NOTE: 'curl' not found, skipping readiness wait.
  start http://localhost:4000/health
  start http://localhost:6540/server-hub/
  goto done
)
echo [4/4] Waiting for services to become ready (up to ~2 min each)...
set /a tries=0
:wait_backend
curl -sf http://localhost:4000/health >nul 2>nul
if %errorlevel%==0 goto backend_ready
set /a tries+=1
if %tries% gtr 60 (
  echo WARNING: backend did not answer in time. Check the backend window, then open URLs manually.
  goto open_anyway
)
echo   backend not up yet... retry %tries%/60
REM ping delay instead of timeout: works even when stdin is not a console.
ping -n 3 127.0.0.1 >nul
goto wait_backend
:backend_ready
echo   backend OK: http://localhost:4000/health
set /a tries=0
:wait_frontend
curl -sf http://localhost:6540/server-hub/ >nul 2>nul
if %errorlevel%==0 goto all_ready
set /a tries+=1
if %tries% gtr 60 (
  echo WARNING: frontend did not answer in time. Check the frontend window, then open URLs manually.
  goto open_anyway
)
echo   frontend not up yet... retry %tries%/60
ping -n 3 127.0.0.1 >nul
goto wait_frontend
:all_ready
echo.
echo All services are up.
:open_anyway
start http://localhost:4000/health
start http://localhost:6540/server-hub/
echo Browsers opened.
:done
endlocal
pause
