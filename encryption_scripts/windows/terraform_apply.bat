@echo off
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0terraform_wrapper.ps1" apply %*
exit /b %ERRORLEVEL%
