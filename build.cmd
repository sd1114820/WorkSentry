@echo off
setlocal
cd /d "%~dp0"

chcp 65001 >nul
set "LOGDIR=%TEMP%\WorkSentry"
if not exist "%LOGDIR%" mkdir "%LOGDIR%" >nul 2>&1
set "LOG=%LOGDIR%\build.log"
if exist "%LOG%" del /f /q "%LOG%" >nul 2>&1

where /q pwsh
if %errorlevel%==0 (
  set "PSH=pwsh"
) else (
  where /q powershell
  if %errorlevel%==0 (
    set "PSH=powershell"
  ) else (
    echo 未找到 PowerShell（pwsh 或 powershell）。
    pause
    exit /b 1
  )
)

if "%~1"=="" (
  rem 交互模式：不重定向输出，否则菜单可能看不到
  "%PSH%" -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1"
) else (
  echo [build] log: %LOG%
  echo 使用 PowerShell: %PSH%>>"%LOG%"
  "%PSH%" -NoProfile -ExecutionPolicy Bypass -File "%~dp0build.ps1" %* >>"%LOG%" 2>>&1
  type "%LOG%"
)

if errorlevel 1 (
  echo.
  echo 构建失败，请查看上方输出。
  pause
  exit /b 1
)

echo.
echo 构建完成。
pause

