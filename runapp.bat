@echo off
setlocal enabledelayedexpansion

for %%i in ("%~dp0.") do SET "script_dir=%%~fi"
cd "%script_dir%"
set "WINLOGINAPP=1"
call run.bat %*
