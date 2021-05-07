:: This program installs builds run.exe and makes it available on the commandline.

:: Prevent output and allow for cmd extensions.
@echo off
setlocal enableextensions enabledelayedexpansion

:: Do not use quotes, needed when calling the commands.
set RUN_EXE_NAME=run.exe
set RUN_DIRPATH=%ProgramFiles%\Liamvdv\Run

set RUN_FILEPATH=%RUN_DIRPATH%\%RUN_EXE_NAME%

:: First, check for priviledges, abort if not elevated
net session >nul 2>&1
if '%ERRORLEVEL%' neq '0' (
    echo.
    echo This script must be run as administrator.
    echo Select "Run as administrator" or type:
    echo     runas /noprofile /user:administrator "cmd.exe /k %~dp0\setup.bat"
    goto :eof
)
:: We have elevated priviledges and thus can continue the install

:: Check if it we are reinstalling, 
if exist "%RUN_FILEPATH%" (
    echo Reinstalling...
    del "%RUN_FILEPATH%"
) else (
    echo Creating folders...

    :: 1) make the missing directories under C:\"Program Files"\Liamvdv\Run
    md "%RUN_DIRPATH%"
)


:: 2) build the executable in the current directory
go build -o %~dp0\run.exe %~dp0\main.go %~dp0\cmd.go

:: Block mkdir and go build is done.
:waittofinish
timeout /t 1 /nobreak >nul 2>&1
if exist "%RUN_DIRPATH%" (
    echo Infinite
    if exist "%~dp0\run.exe" (
       goto cont
    )
)
goto waittofinish

:cont
:: 3) copy the file run.exe from the current directory to C:\Program Files\Liamvdv\Run\run.exe 
move /Y "%~dp0\run.exe" "%RUN_DIRPATH%"

:: 4) If all these commands succeded, add the dir to PATH to access the cmd from anywhere.
:: Prompt the user
echo Executables that are in PATH can be executed from anywhere. You most likely want to type: y
set SETXPATH=n
set /P SETXPATH=Do you want to add run to the PATH? (y/n)
if /I "%SETXPATH%" neq "y" goto end

setx /M PATH "%PATH%;%RUN_DIRPATH%"

:end
echo Done.
run -list