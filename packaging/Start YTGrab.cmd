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
ytgrab.exe --open
if errorlevel 1 pause
