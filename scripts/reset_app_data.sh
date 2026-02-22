#!/usr/bin/env sh
set -eu

# Resets Briefcast runtime data while preserving configuration files.
# - Stops compose services
# - Clears database records (Postgres) or SQLite files
# - Clears assets and logs directories
# - Removes generated backups
# - Optionally starts services again

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$PROJECT_DIR/docker-compose.yml"
ENV_FILE="$PROJECT_DIR/.env"
AUTO_CONFIRM="false"
START_AFTER_RESET="true"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/reset_app_data.sh [options]

Options:
  --env-file <path>   Path to env file (default: ./.env)
  --no-start          Do not start services after reset
  --yes               Skip interactive confirmation
  -h, --help          Show this help

Example:
  ./scripts/reset_app_data.sh --env-file /volume1/docker/podcasts-briefcast/.env --yes
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --env-file)
      if [ $# -lt 2 ]; then
        echo "error: --env-file requires a value" >&2
        exit 1
      fi
      ENV_FILE="$2"
      shift 2
      ;;
    --no-start)
      START_AFTER_RESET="false"
      shift
      ;;
    --yes)
      AUTO_CONFIRM="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [ ! -f "$ENV_FILE" ]; then
  echo "error: env file not found: $ENV_FILE" >&2
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "error: compose file not found: $COMPOSE_FILE" >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE_IMPL="docker compose"
elif command -v docker-compose >/dev/null 2>&1; then
  COMPOSE_IMPL="docker-compose"
else
  echo "error: docker compose is not available" >&2
  exit 1
fi

compose() {
  if [ "$COMPOSE_IMPL" = "docker compose" ]; then
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
  else
    docker-compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
  fi
}

env_get() {
  key="$1"
  fallback="$2"
  value=$(sed -n "s/^[[:space:]]*$key[[:space:]]*=[[:space:]]*//p" "$ENV_FILE" | tail -n 1 | tr -d '\r')
  if [ -z "$value" ]; then
    printf '%s' "$fallback"
    return
  fi
  case "$value" in
    \"*\")
      value=${value#\"}
      value=${value%\"}
      ;;
    \'*\')
      value=${value#\'}
      value=${value%\'}
      ;;
  esac
  printf '%s' "$value"
}

resolve_path() {
  raw="$1"
  if [ -z "$raw" ]; then
    printf '%s' "$PROJECT_DIR"
    return
  fi
  case "$raw" in
    /*)
      printf '%s' "$raw"
      ;;
    ~)
      printf '%s' "$HOME"
      ;;
    ~/*)
      printf '%s/%s' "$HOME" "${raw#~/}"
      ;;
    .)
      printf '%s' "$PROJECT_DIR"
      ;;
    ./*)
      printf '%s/%s' "$PROJECT_DIR" "${raw#./}"
      ;;
    *)
      printf '%s/%s' "$PROJECT_DIR" "$raw"
      ;;
  esac
}

safe_clear_directory() {
  target="$1"
  label="$2"
  if [ -z "$target" ] || [ "$target" = "/" ]; then
    echo "error: refusing to clear unsafe path for $label: '$target'" >&2
    exit 1
  fi
  mkdir -p "$target"
  find "$target" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
}

DB_DRIVER=$(env_get "DB_DRIVER" "")
if [ -z "$DB_DRIVER" ]; then
  DB_DRIVER=$(env_get "DATABASE_DRIVER" "")
fi
DATABASE_URL=$(env_get "DATABASE_URL" "")
if [ -z "$DB_DRIVER" ]; then
  case "$DATABASE_URL" in
    postgres://*|postgresql://*)
      DB_DRIVER="postgres"
      ;;
    *)
      DB_DRIVER="sqlite"
      ;;
  esac
fi

HOST_CONFIG_DIR=$(resolve_path "$(env_get "HOST_CONFIG_DIR" "./config")")
HOST_ASSETS_DIR=$(resolve_path "$(env_get "HOST_ASSETS_DIR" "./assets")")
HOST_LOGS_DIR=$(resolve_path "$(env_get "HOST_LOGS_DIR" "./logs")")
WHISPERX_ENV_FILE=$(resolve_path "$(env_get "WHISPERX_ENV_FILE" ".env.whisperx")")

echo "Reset plan:"
echo "  project: $PROJECT_DIR"
echo "  env file: $ENV_FILE"
echo "  compose: $COMPOSE_IMPL"
echo "  db driver: $DB_DRIVER"
echo "  config dir: $HOST_CONFIG_DIR"
echo "  assets dir: $HOST_ASSETS_DIR"
echo "  logs dir: $HOST_LOGS_DIR"
echo "  whisperx env file: $WHISPERX_ENV_FILE"
echo
echo "This will remove transactional data (DB records, assets, logs, backups)."
echo "Configuration files are preserved."

if [ "$AUTO_CONFIRM" != "true" ]; then
  printf "Type RESET to continue: "
  read -r reply
  if [ "$reply" != "RESET" ]; then
    echo "aborted"
    exit 1
  fi
fi

echo "Stopping services..."
if [ ! -f "$WHISPERX_ENV_FILE" ]; then
  mkdir -p "$(dirname "$WHISPERX_ENV_FILE")"
  : > "$WHISPERX_ENV_FILE"
fi
compose down --remove-orphans || true

if [ "$DB_DRIVER" = "postgres" ]; then
  if [ -z "$DATABASE_URL" ]; then
    echo "error: DATABASE_URL is required for postgres resets" >&2
    exit 1
  fi
  echo "Clearing Postgres tables..."
  SQL="TRUNCATE TABLE IF EXISTS podcast_tags,podcast_items,podcasts,tags,settings,migrations,job_locks RESTART IDENTITY CASCADE;"
  docker run --rm --network host postgres:17-alpine \
    psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -c "$SQL"
else
  echo "Removing SQLite database files..."
  rm -f "$HOST_CONFIG_DIR/briefcast.db" \
        "$HOST_CONFIG_DIR/briefcast.db-shm" \
        "$HOST_CONFIG_DIR/briefcast.db-wal"
fi

echo "Removing generated backups..."
rm -rf "$HOST_CONFIG_DIR/backups"
mkdir -p "$HOST_CONFIG_DIR/backups"

echo "Clearing assets and logs..."
safe_clear_directory "$HOST_ASSETS_DIR" "assets"
safe_clear_directory "$HOST_LOGS_DIR" "logs"

if [ "$START_AFTER_RESET" = "true" ]; then
  echo "Starting services..."
  compose up -d --force-recreate
  echo "Reset complete and services restarted."
else
  echo "Reset complete. Services are stopped (--no-start)."
fi
