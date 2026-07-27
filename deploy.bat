@echo off
cd /d "C:\Users\Windows\AppData\Roaming\TRAE SOLO CN\ModularData\ai-agent\work-mode-projects\6a651e53f646f6881ff62012\cb-platform"
echo [1/3] Copying binary to container...
docker cp cb-platform-server.exe cb-platform-app:/app/cb-platform
if errorlevel 1 (
    echo [FAIL] docker cp failed
    exit /b 1
)
echo [2/3] Restarting container...
docker restart cb-platform-app
if errorlevel 1 (
    echo [FAIL] docker restart failed
    exit /b 1
)
echo [3/3] Waiting for healthcheck...
timeout /t 8 /nobreak >nul
docker ps --filter "name=cb-platform-app" --format "{{.Names}} {{.Status}}"
