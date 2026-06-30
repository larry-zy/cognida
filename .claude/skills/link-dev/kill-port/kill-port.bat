@echo off
REM Kill process on specified port (Windows)

IF "%1"=="" (
    echo Usage: kill-port.bat ^<port^>
    exit /b 1
)

SET PORT=%1

echo Checking for process on port %PORT%...

FOR /F "tokens=5" %%P IN ('netstat -ano ^| findstr :%PORT% ^| findstr LISTENING') DO (
    echo Found process %%P on port %PORT%
    echo Killing process...
    taskkill /F /PID %%P
    IF %ERRORLEVEL% EQU 0 (
        echo Process killed successfully
    ) ELSE (
        echo Failed to kill process
    )
    exit /b 0
)

echo No process found on port %PORT%
exit /b 0
