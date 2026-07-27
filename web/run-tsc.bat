@echo off
cd /d "C:\Users\Windows\AppData\Roaming\TRAE SOLO CN\ModularData\ai-agent\work-mode-projects\6a651e53f646f6881ff62012\cb-platform\web"
call npx tsc --noEmit -p tsconfig.json
exit /b %ERRORLEVEL%
