#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
python3 docker/deploy_test.py
run="samen-klaar-flow-$$"
cleanup() {
  docker rm -f "$run-app" "$run-db" >/dev/null 2>&1 || true
  docker network rm "$run" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM
docker build --file docker/Dockerfile --target verification --tag samen-klaar-flow-check:local .
docker network create "$run" >/dev/null
# Disposable database, never the running application's database or persistent volume.
docker run -d --name "$run-db" --network "$run" --network-alias db \
  --tmpfs /var/lib/postgresql/data \
  -e POSTGRES_DB=samenklaar -e POSTGRES_USER=samenklaar -e POSTGRES_PASSWORD=flow-test-only \
  postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea >/dev/null
attempt=0
until docker exec "$run-db" pg_isready -U samenklaar -d samenklaar >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -gt 30 ]; then echo 'Verification database failed to start' >&2; exit 1; fi
  sleep 1
done
docker run --rm --name "$run-app" --network "$run" \
  -e TEST_DATABASE_URL=postgres://samenklaar:flow-test-only@db:5432/samenklaar?sslmode=disable \
  -e WEB_DIR=/app/dist \
  samen-klaar-flow-check:local go test -p 1 -count=1 -v ./...
