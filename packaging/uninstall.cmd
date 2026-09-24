@echo off
setlocal enabledelayedexpansion
title QuickFlare Uninstaller

echo ======================================================
echo              QuickFlare Uninstaller
echo ======================================================
echo.

set "AUTO_CONFIRM=0"
if /i "%~1"=="/y" set "AUTO_CONFIRM=1"
if /i "%~1"=="-y" set "AUTO_CONFIRM=1"
if /i "%~1"=="/quiet" set "AUTO_CONFIRM=1"

if "!AUTO_CONFIRM!"=="0" (
    set /p "CONFIRM=Are you sure you want to uninstall QuickFlare? (Y/N): "
    if /i not "!CONFIRM!"=="Y" (
        echo Uninstallation cancelled.
        exit /b 0
    )
)

echo.
echo Stopping any running QuickFlare processes...
if exist "%~dp0quickflare.exe" (
    "%~dp0quickflare.exe" stop >nul 2>&1
)
taskkill /f /im quickflare-tray.exe >nul 2>&1
taskkill /f /im quickflare.exe >nul 2>&1
taskkill /f /im cloudflared.exe >nul 2>&1

set "PRODUCT_CODE="
for /f "tokens=2*" %%a in ('reg query "HKCU\Software\QuickFlare" /v ProductCode 2^>nul') do (
    set "PRODUCT_CODE=%%b"
)

if not "%PRODUCT_CODE%"=="" (
    echo Found Windows Installer package (%PRODUCT_CODE%).
    echo Launching Windows uninstaller...
    start "" msiexec.exe /x %PRODUCT_CODE%
    exit /b 0
)

echo Windows Installer product code not found in registry.
echo Performing manual cleanup...
if exist "%~dp0quickflare.exe" (
    "%~dp0quickflare.exe" path remove
)
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare.exe" /f >nul 2>&1
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\App Paths\quickflare-tray.exe" /f >nul 2>&1
reg delete "HKCU\Software\QuickFlare" /f >nul 2>&1

echo.
echo QuickFlare PATH and registry associations removed.
echo You may now delete this folder: "%~dp0"
pause
exit /b 0
