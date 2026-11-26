@echo off
setlocal

set "KIT_DIR=%~dp0"
set "INSTANCE=school-tracker"
set "INSTANCE_DIR=%KIT_DIR%worker\instances\%INSTANCE%"

if not exist "%INSTANCE_DIR%\package.json" (
  echo Worker instance not found: %INSTANCE_DIR%
  pause
  exit /b 1
)

:menu
cls
echo Cloudflare D1 - %INSTANCE%
echo.
echo   1. Assemble worker from shared template
echo   2. Typecheck worker
echo   3. Run worker locally
echo   4. Deploy worker
echo   5. Preview infrastructure provisioning
echo   6. List cloud accounts
echo   7. Export an existing connection key
echo   8. Check Wrangler login
echo   9. Exit
echo.
choice /C 123456789 /N /M "Select a command: "
if errorlevel 9 exit /b 0
if errorlevel 8 goto :wrangler_login
if errorlevel 7 goto :export_key
if errorlevel 6 goto :admin_list
if errorlevel 5 goto :provision_preview
if errorlevel 4 goto :deploy
if errorlevel 3 goto :dev
if errorlevel 2 goto :typecheck
if errorlevel 1 goto :assemble
goto :menu

:assemble
call :assemble_instance
if errorlevel 1 goto :failed
echo.
echo Worker assembled successfully.
pause
goto :menu

:typecheck
pushd "%INSTANCE_DIR%"
call npm run typecheck
set "RESULT=%ERRORLEVEL%"
popd
if not "%RESULT%"=="0" goto :failed
echo.
echo Typecheck passed.
pause
goto :menu

:dev
pushd "%INSTANCE_DIR%"
call npm run dev
set "RESULT=%ERRORLEVEL%"
popd
if not "%RESULT%"=="0" goto :failed
pause
goto :menu

:deploy
call :assemble_instance
if errorlevel 1 goto :failed

pushd "%INSTANCE_DIR%"
call npm run typecheck
if errorlevel 1 goto :deploy_typecheck_failed

echo.
echo This will deploy the %INSTANCE% worker to Cloudflare.
choice /C YN /N /M "Continue with production deploy? [Y/N] "
if errorlevel 2 (
  popd
  echo Deployment cancelled.
  pause
  goto :menu
)

call npm run deploy
set "RESULT=%ERRORLEVEL%"
popd
if not "%RESULT%"=="0" goto :failed
echo.
echo Deployment completed.
pause
goto :menu

:provision_preview
node "%KIT_DIR%scripts\provision.mjs" --dir "%INSTANCE_DIR%" --dry-run
if errorlevel 1 goto :failed
pause
goto :menu

:admin_list
echo Requires SYNC_ADMIN_ID_TOKEN to be set in this terminal.
node "%KIT_DIR%scripts\admin.mjs" --dir "%INSTANCE_DIR%" list
if errorlevel 1 goto :failed
pause
goto :menu

:export_key
echo The exported .key file contains a sync credential. Keep it private.
node "%KIT_DIR%scripts\generate-key.mjs" --dir "%INSTANCE_DIR%"
if errorlevel 1 goto :failed
pause
goto :menu

:wrangler_login
pushd "%INSTANCE_DIR%"
call npx wrangler whoami
set "RESULT=%ERRORLEVEL%"
popd
if not "%RESULT%"=="0" goto :failed
pause
goto :menu

:assemble_instance
pushd "%KIT_DIR%"
node scripts\build-instance.mjs %INSTANCE%
set "RESULT=%ERRORLEVEL%"
popd
exit /b %RESULT%

:failed
echo.
echo Command failed. Review the output above.
pause
goto :menu

:deploy_typecheck_failed
set "RESULT=%ERRORLEVEL%"
popd
goto :failed
