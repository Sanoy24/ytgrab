@echo off
rem Starts YTGrab from this folder and opens it in the default browser.
cd /d "%~dp0"
ytgrab.exe --open
if errorlevel 1 pause
