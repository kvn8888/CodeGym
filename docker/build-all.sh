#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "Building CodeGym runtime images..."

# Each image gets the shared entrypoint script
for dir in base go122 node20 node20-express python312; do
    echo ""
    echo "=== Building codegym/${dir} ==="
    cp "$SCRIPT_DIR/codegym-run.sh" "$SCRIPT_DIR/${dir}/codegym-run.sh"
    docker build -t "codegym/${dir}" "$SCRIPT_DIR/${dir}"
    rm -f "$SCRIPT_DIR/${dir}/codegym-run.sh"
done

echo ""
echo "All images built successfully!"
docker images | grep codegym
