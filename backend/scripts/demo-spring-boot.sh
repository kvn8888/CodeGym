#!/bin/bash
set -e

# Demo script for the Spring Boot deterministic fixture demo
# Runs the full environment pipeline: request → build → verify → open practice session
# No model credentials or network required (uses pinned fixture and offline Maven cache).

cd "$(dirname "$0")/.."

# Path to opencode binary (not required for fixture mode, but keep for live switch)
export CODEGYM_OPENCODE_BINARY="${CODEGYM_OPENCODE_BINARY:-/c/Users/hunte/AppData/Roaming/npm/opencode}"

# Add Maven and Git sh to PATH
export PATH="/c/Program Files/apache-maven-3.9.6/bin:/c/Program Files/Git/usr/bin:$PATH"

# Env-demo settings
export CODEGYM_ENV_DEMO=1
export CODEGYM_ENV_DEMO_FIXTURE=1
export CODEGYM_ASSESSMENT_INPUTS_FILE="config/spring-boot.assessment.json"
export CODEGYM_ALLOW_UNCACHED_SETUP=1

# Dev settings
export CODEGYM_AUTH_MODE=dev
export CODEGYM_DEV_AUTH_TOKEN="${CODEGYM_DEV_AUTH_TOKEN:-dev-demo-token}"
export CODEGYM_DEV_USER_ID="${CODEGYM_DEV_USER_ID:-demo-user}"
export CODEGYM_DEV_WORKSPACE_ID="${CODEGYM_DEV_WORKSPACE_ID:-demo-workspace}"
export CODEGYM_PORT="${CODEGYM_PORT:-8080}"
export CODEGYM_HOST="127.0.0.1"
export CODEGYM_CORS_ALLOWED_ORIGINS="http://localhost:3000,http://127.0.0.1:3000,http://localhost:5173,http://127.0.0.1:5173"

# Relay not required for fixture, but set a dummy secret if referenced
export CODEGYM_RELAY_TOKEN_SECRET="${CODEGYM_RELAY_TOKEN_SECRET:-demo-secret}"

echo "Starting CodeGym Spring Boot fixture demo..."
echo "  Endpoint: http://127.0.0.1:$CODEGYM_PORT"
echo "  Assessment file: $CODEGYM_ASSESSMENT_INPUTS_FILE"
echo ""

go run ./cmd/server
