@echo off
rem Starts YTGrab from this folder. If something it needs is missing, runs setup first.
cd /d "%~dp0"
ytgrab.exe doctor >NUL 2>&1
if errorlevel 1 (
  echo Some things YTGrab needs are missing. Starting setup...
  echo.
  ytgrab.exe setup
  echo.
)
rem "start" gives YTGrab its own window, which it hides; look for its icon by the clock.
start "" ytgrab.exe --open
