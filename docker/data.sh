#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
case "${1:-}" in
  seed-pros|import-seed-pros-auth0) ;;
  *) echo 'Usage: docker/data.sh seed-pros|import-seed-pros-auth0' >&2; exit 1 ;;
esac
test -f .deploy/local.env || { echo 'Run make deploy-local first.' >&2; exit 1; }
test -f .secrets/local/seed.env || { echo 'Create .secrets/local/seed.env with SEED_PASSWORD first.' >&2; exit 1; }
mkdir -p .secrets/local/seed-pros
chmod 700 .secrets/local/seed-pros
docker compose --project-directory . --env-file .deploy/local.env \
  -f docker/compose.yaml -f docker/compose.local.yaml \
  run --rm --no-deps --user "$(id -u):$(id -g)" \
  -e SEED_CONFIG_FILE=/run/secrets/seed.env -e SEED_OUTPUT_DIR=/seed-output \
  -v "$PWD/.secrets/local/seed.env:/run/secrets/seed.env:ro" \
  -v "$PWD/.secrets/local/seed-pros:/seed-output" app "$1"
