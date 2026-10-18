#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
action=${1:-help}
mode=${2:-local}
case "$mode" in local|prod) ;; *) echo 'Environment must be local or prod' >&2; exit 1 ;; esac
DEPLOY_ENV=$mode
if [ "$mode" = prod ]; then APP_ENV=production; else APP_ENV=development; fi
export DEPLOY_ENV APP_ENV
state=".deploy/$mode"
config=".deploy/$mode.env"
secrets=".secrets/$mode"

fail() { echo "$*" >&2; exit 1; }
setting() { awk -F= -v key="$1" '$1 == key { print substr($0, length(key) + 2); exit }' "$config"; }
compose() {
  CADDY_FILE=${CADDY_FILE:-$(setting CADDY_FILE)}
  case "$CADDY_FILE" in "./deploy/Caddyfile.$mode") CADDY_FILE="./docker/Caddyfile.$mode" ;; esac
  export CADDY_FILE
  docker compose --project-directory . --env-file "$config" -f docker/compose.yaml -f "docker/compose.$mode.yaml" "$@"
}

# The untracked file that seeds an environment's app settings: .env locally, .env.production in production.
settings_file() { if [ "$mode" = prod ]; then echo .env.production; else echo .env; fi; }

# One value from the production settings file (DOMAIN, ACME_EMAIL, APP_IMAGE); empty when absent.
production_value() {
  [ -f .env.production ] || return 0
  awk -F= -v key="$1" '$1 == key { print substr($0, length(key) + 2); exit }' .env.production
}

import_local_settings() {
  source_file=$(settings_file)
  [ -f "$source_file" ] || return 0
  # Only fill empty settings on an existing deployment. Keep its database URL,
  # browser origin and callback, and never print provider credentials.
  umask 077
  import_file=$(mktemp "$secrets/app.env.XXXXXX")
  awk -v replace_existing="${1:-false}" '
    function empty(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value == "" || value == "\"\"" }
    FILENAME == ARGV[1] { pos=index($0,"="); if(pos>0) values[substr($0,1,pos-1)]=substr($0,pos+1); next }
    { pos=index($0,"="); key=substr($0,1,pos-1); value=substr($0,pos+1);
      if(pos>0 && key!="DATABASE_URL" && key!="APP_BASE_URL" && key!="AUTH0_REDIRECT_URI" && !empty(values[key]) && (replace_existing=="true" || empty(value))) print key "=" values[key]; else print }
  ' "$source_file" "$secrets/app.env" > "$import_file"
  if cmp -s "$import_file" "$secrets/app.env"; then
    rm "$import_file"
  else
    chmod 644 "$import_file"
    mv "$import_file" "$secrets/app.env"
  fi
}

init() {
  umask 077
  mkdir -p "$state" "$secrets"
  chmod 700 .deploy .secrets "$state" "$secrets"
  if [ "$mode" = prod ]; then
    # Command-line values win; .env.production fills the rest; the placeholders make deploy refuse to run.
    domain=${DOMAIN:-$(production_value DOMAIN)}
    domain=${domain:-app.example.com}
    email=${ACME_EMAIL:-$(production_value ACME_EMAIL)}
    email=${email:-you@example.com}
    printf '%s\n' "$domain" | grep -Eq '^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$' || fail 'DOMAIN must be a hostname'
    printf '%s\n' "$email" | grep -Eq '^[a-zA-Z0-9._+%-]+@[a-zA-Z0-9.-]+$' || fail 'ACME_EMAIL must be an email address'
    base="https://$domain"
    app_env=production
    image=${APP_IMAGE:-$(production_value APP_IMAGE)}
    image=${image:-registry.example.com/samen-klaar@sha256:REPLACE_WITH_RELEASE_DIGEST}
  else
    domain=localhost
    email=''
    base=http://localhost:8090
    app_env=development
    image=samen-klaar-local:dev
  fi
  if [ ! -f "$secrets/postgres-password" ]; then
    # Do not generate a new password for an existing deployment.
    [ ! -f "$secrets/app.env" ] || fail 'Postgres password file missing; recover it before continuing'
    openssl rand -hex 32 > "$secrets/postgres-password"
  fi
  if [ ! -f "$secrets/app.env" ]; then
    db_password=$(cat "$secrets/postgres-password")
    printf '%s\n' "$db_password" | grep -Eq '^[a-f0-9]{64}$' || fail 'Expected the generated PostgreSQL password; recover the original file'
    sed -e "s|__BASE_URL__|$base|g" -e "s|__DB_PASSWORD__|$db_password|g" docker/app.env.example > "$secrets/app.env"
    import_local_settings true
  fi
  # Compose file secrets are bind mounts; mode/uid settings cannot change their
  # ownership. The non-root app can read the file, while host directories are 700.
  chmod 644 "$secrets/app.env" "$secrets/postgres-password"
  if [ ! -f "$config" ]; then
    cat > "$config" <<EOF
DEPLOY_ENV=$mode
APP_ENV=$app_env
APP_IMAGE=$image
DOMAIN=$domain
ACME_EMAIL=$email
CADDY_FILE=./docker/Caddyfile.$mode
APP_CONFIG_PATH=./$secrets/app.env
POSTGRES_PASSWORD_PATH=./$secrets/postgres-password
EOF
  fi
  import_local_settings
  echo "Configuration ready. Edit $secrets/app.env; deployment settings are in $config."
}

backup() {
  umask 077
  mkdir -p ".backups/$mode"
  backup_path=".backups/$mode/$(date -u +%Y%m%dT%H%M%SZ)-$$.dump"
  if compose exec -T db pg_dump -U samenklaar -d samenklaar --format=custom > "$backup_path.tmp"; then
    mv "$backup_path.tmp" "$backup_path"
    echo "Database backup: $backup_path"
  else
    rm -f "$backup_path.tmp"
    fail 'Database backup failed'
  fi
}

public_check() {
  if [ "$mode" = local ]; then endpoint=http://localhost:8090; else endpoint="https://$(setting DOMAIN)"; fi
  curl --fail --silent --show-error --connect-timeout 5 --max-time 10 \
    --retry 6 --retry-delay 3 --retry-all-errors "$endpoint/api/health/ready" > /dev/null
}

restore_image() {
  APP_IMAGE=$1
  export APP_IMAGE
  compose up -d --no-deps --no-build --wait --wait-timeout 120 app
}

if [ "$action" = init ]; then init; exit 0; fi
if [ ! -f "$config" ]; then
  if [ "$mode" = local ]; then init; else fail 'Run make init-prod and fill in the server configuration first'; fi
fi
command -v docker >/dev/null || fail 'Docker with Compose is required'
docker info > /dev/null 2>&1 || fail 'Docker engine is not running or is not accessible'
APP_IMAGE=${APP_IMAGE:-$(setting APP_IMAGE)}
export APP_IMAGE

case "$action" in
  status) compose ps; exit 0 ;;
  logs) compose logs --tail 100 -f; exit 0 ;;
  stop) compose down --remove-orphans; exit 0 ;;
  backup) backup; exit 0 ;;
  deploy|rollback|infra|migrate|validate) ;;
  *) fail "Unknown action: $action" ;;
esac

# One writer per environment. A stopped deployment leaves an obvious lock that
# can be removed after checking no deployment process is still running.
mkdir "$state/lock" 2>/dev/null || fail "Deployment locked; inspect the running process before removing $state/lock"
trap 'rmdir "$state/lock"' EXIT
trap 'exit 130' HUP INT TERM
case "$action" in deploy|migrate|validate) import_local_settings ;; esac
compose config --quiet

if [ "$action" = infra ]; then
  compose up -d --no-recreate --wait --wait-timeout 120 db
  exit 0
fi

if [ "$action" = rollback ]; then
  [ "$mode" = prod ] || fail 'Rollback is a production command'
  [ -s "$state/previous-image" ] || fail 'No previous successful image is recorded'
  previous=$(cat "$state/previous-image")
  current=$(cat "$state/current-image")
  restore_image "$previous"
  public_check
  printf '%s\n' "$current" > "$state/previous-image"
  printf '%s\n' "$previous" > "$state/current-image"
  echo 'Previous app image restored. Database schema and secrets were preserved.'
  exit 0
fi

if [ "$mode" = local ]; then
  compose build app
else
  printf '%s\n' "$APP_IMAGE" | grep -Eq '^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$' || fail 'Production APP_IMAGE must include an immutable sha256 digest'
  [ "$(setting DOMAIN)" != app.example.com ] || fail 'Set the real DOMAIN in .deploy/prod.env'
  [ "$(setting ACME_EMAIL)" != you@example.com ] || fail 'Set the real ACME_EMAIL in .deploy/prod.env'
  compose pull app gateway db
fi

# Preflight never starts HTTP, workers, or migrations.
if [ "$mode" = local ]; then expected_origin=http://localhost:8090; else expected_origin="https://$(setting DOMAIN)"; fi
if ! compose run --rm --no-deps -e "EXPECTED_APP_BASE_URL=$expected_origin" migrate check-config; then
  fail "Configuration check failed. Edit the active app configuration: $(setting APP_CONFIG_PATH)"
fi
compose run --rm --no-deps gateway caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
if [ "$action" = validate ]; then exit 0; fi

# Application releases do not recreate or upgrade an existing database container.
compose up -d --no-recreate --wait --wait-timeout 120 db
if [ "$mode" = prod ]; then backup; fi
compose run --rm --no-deps migrate
if [ "$action" = migrate ]; then exit 0; fi

old_image=''
if [ "$mode" = prod ] && [ -s "$state/current-image" ]; then old_image=$(cat "$state/current-image"); fi
new_image=$APP_IMAGE
if compose up -d --no-deps --no-build --wait --wait-timeout 120 app && \
   compose up -d --no-deps --wait --wait-timeout 90 gateway && \
   compose exec -T gateway caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile && \
   public_check; then
  if [ "$mode" = prod ]; then
    if [ -n "$old_image" ] && [ "$old_image" != "$new_image" ]; then printf '%s\n' "$old_image" > "$state/previous-image"; fi
    printf '%s\n' "$new_image" > "$state/current-image.tmp"
    mv "$state/current-image.tmp" "$state/current-image"
  fi
  echo "Deployment healthy ($mode)."
else
  echo 'Deployment failed. Inspect make status/logs for this environment.' >&2
  if [ -n "$old_image" ]; then
    if restore_image "$old_image" && public_check; then
      echo 'Previous app image restored; deployment still reports failure.' >&2
    else
      echo 'Automatic rollback also failed; inspect the app, gateway, and database.' >&2
    fi
  fi
  exit 1
fi
