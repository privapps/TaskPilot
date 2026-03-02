@echo off
REM Test script for TaskPilot - runs all unit tests

echo Running TaskPilot unit tests...

REM Parse command line options
set VERBOSE=
set RUN_PATTERN=
set COVER_FLAG=

:parse_args
if "%1"=="" goto run_tests
if "%1"=="-v" set VERBOSE=-v& shift & goto parse_args
if "%1"=="--verbose" set VERBOSE=-v& shift & goto parse_args
if "%1"=="-run" set RUN_PATTERN=-run %2& shift & shift & goto parse_args
if "%1"=="-cover" set COVER_FLAG=-cover& shift & goto parse_args
if "%1"=="--coverage" set COVER_FLAG=-cover& shift & goto parse_args
echo Unknown option: %1
echo Usage: %0 [-v^|--verbose] [-run pattern] [-cover^|--coverage]
exit /b 1

:run_tests
go test %VERBOSE% %RUN_PATTERN% %COVER_FLAG% ./...

if %ERRORLEVEL% EQU 0 (
  echo [32m✓ All tests passed![0m
  exit /b 0
) else (
  echo [31m✗ Tests failed[0m
  exit /b 1
)
