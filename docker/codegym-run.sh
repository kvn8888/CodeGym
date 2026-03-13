#!/bin/bash
set -e

# CodeGym container entrypoint
# Environment variables:
#   CODEGYM_BUILD_CMD  - optional build command (e.g., "npm install")
#   CODEGYM_TEST_CMD   - test command to execute (e.g., "go test -v -json ./...")
#   CODEGYM_SETUP_CMDS - newline-separated setup commands (e.g., start server)

cd /app/workspace

# Run build command if specified
if [ -n "$CODEGYM_BUILD_CMD" ]; then
    echo "::codegym::build_start"
    eval "$CODEGYM_BUILD_CMD"
    echo "::codegym::build_end"
fi

# Run setup commands if specified (e.g., start a server in background)
if [ -n "$CODEGYM_SETUP_CMDS" ]; then
    echo "::codegym::setup_start"
    while IFS= read -r cmd; do
        [ -z "$cmd" ] && continue
        eval "$cmd"
    done <<< "$CODEGYM_SETUP_CMDS"
    echo "::codegym::setup_end"
fi

# Run the test command
echo "::codegym::test_start"
eval "$CODEGYM_TEST_CMD"
TEST_EXIT=$?
echo "::codegym::test_end"

exit $TEST_EXIT
