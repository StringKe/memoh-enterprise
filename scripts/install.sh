#!/bin/sh
set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
PURPLE='\033[0;35m'
RED='\033[0;31m'
NC='\033[0m'

GITHUB_REPO="StringKe/memoh-enterprise"
REPO="https://github.com/${GITHUB_REPO}.git"
DIR="memoh-enterprise"
COMPOSE_PROJECT_NAME="memoh"
SILENT=false
DRY_RUN=false
DEPLOY_RUNTIME="${MEMOH_DEPLOY_RUNTIME:-containerd}"
VERSION_SET=false

# Track whether the user explicitly set environment-backed options so upgrades
# can reuse prior install values by default.
if [ "${MEMOH_INSTALL_MODE+x}" = x ]; then
  INSTALL_MODE="$MEMOH_INSTALL_MODE"
else
  INSTALL_MODE="auto"
fi
if [ "${MEMOH_DATABASE_DRIVER+x}" = x ]; then
  DATABASE_DRIVER="$MEMOH_DATABASE_DRIVER"
  DATABASE_DRIVER_SET=true
else
  DATABASE_DRIVER="postgres"
  DATABASE_DRIVER_SET=false
fi
if [ "${MEMOH_CONTAINER_BACKEND+x}" = x ]; then
  CONTAINER_BACKEND="$MEMOH_CONTAINER_BACKEND"
  CONTAINER_BACKEND_SET=true
else
  CONTAINER_BACKEND="containerd"
  CONTAINER_BACKEND_SET=false
fi
if [ "${USE_SPARSE+x}" = x ]; then
  USE_SPARSE_SET=true
else
  USE_SPARSE_SET=false
fi
if [ "${BROWSER_CORE+x}" = x ]; then
  BROWSER_CORE_SET=true
else
  BROWSER_CORE_SET=false
fi

NETWORK_NAME="${COMPOSE_PROJECT_NAME}_memoh-network"
PROJECT_CONTAINERS="memoh-postgres memoh-migrate memoh-server memoh-web memoh-browser memoh-agent-runner memoh-connector memoh-integration-gateway memoh-worker memoh-sparse memoh-qdrant"
PROJECT_VOLUMES="${COMPOSE_PROJECT_NAME}_postgres_data ${COMPOSE_PROJECT_NAME}_containerd_data ${COMPOSE_PROJECT_NAME}_memoh_data ${COMPOSE_PROJECT_NAME}_server_cni_state ${COMPOSE_PROJECT_NAME}_qdrant_data ${COMPOSE_PROJECT_NAME}_openviking_data"

EXISTING_CONFIG_SOURCE=""
EXISTING_ENV_SOURCE=""
EXISTING_INSTALL_STATE=false
EXISTING_DOCKER_STATE=false
EXISTING_DOCKER_VOLUMES=""
EXISTING_DOCKER_CONTAINERS=false
EXISTING_DOCKER_NETWORK=false
EXISTING_WORKSPACE_FILES=false
EXISTING_REPO_DIR=false

# Parse flags
while [ $# -gt 0 ]; do
  case "$1" in
    -y|--yes) SILENT=true ;;
    --dry-run) DRY_RUN=true ;;
    --version)
      shift
      MEMOH_VERSION="$1"
      VERSION_SET=true
      ;;
    --version=*)
      MEMOH_VERSION="${1#--version=}"
      VERSION_SET=true
      ;;
    --runtime)
      shift
      DEPLOY_RUNTIME="$1"
      ;;
    --runtime=*)
      DEPLOY_RUNTIME="${1#--runtime=}"
      ;;
    --install-mode)
      shift
      INSTALL_MODE="$1"
      ;;
    --install-mode=*)
      INSTALL_MODE="${1#--install-mode=}"
      ;;
    --database-driver)
      shift
      DATABASE_DRIVER="$1"
      DATABASE_DRIVER_SET=true
      ;;
    --database-driver=*)
      DATABASE_DRIVER="${1#--database-driver=}"
      DATABASE_DRIVER_SET=true
      ;;
    --container-backend|--workspace-backend)
      shift
      CONTAINER_BACKEND="$1"
      CONTAINER_BACKEND_SET=true
      ;;
    --container-backend=*|--workspace-backend=*)
      CONTAINER_BACKEND="${1#*=}"
      CONTAINER_BACKEND_SET=true
      ;;
  esac
  shift
done

# Auto-silent if no TTY available
if [ "$SILENT" = false ] && ! [ -e /dev/tty ]; then
  SILENT=true
fi

echo "${PURPLE}Memoh One-Click Install${NC}"

if [ "$(id -u 2>/dev/null || printf '1')" = "0" ] && [ "${MEMOH_ALLOW_ROOT_INSTALL:-false}" != "true" ]; then
  echo "${RED}Error: Do not run this installer as root.${NC}"
  echo "Run it as your normal user instead:"
  echo "  curl -fsSL https://memoh.sh | sh"
  echo ""
  echo "The installer will use sudo for package installation and runtime commands when required."
  echo "To override this guard, set MEMOH_ALLOW_ROOT_INSTALL=true."
  exit 1
fi

read_env_file_value() {
  file="$1"
  key="$2"
  if [ ! -f "$file" ]; then
    return 1
  fi
  value=$(grep "^${key}=" "$file" 2>/dev/null | tail -n 1 | cut -d '=' -f 2-)
  if [ -z "$value" ]; then
    return 1
  fi
  case "$value" in
    \'*\')
      value=${value#\'}
      value=${value%\'}
      value=$(printf '%s' "$value" | sed "s/\\\\'/'/g")
      ;;
  esac
  printf '%s' "$value"
}

read_toml_value() {
  file="$1"
  section="$2"
  key="$3"
  if [ ! -f "$file" ]; then
    return 1
  fi
  value=$(awk -v target_section="[$section]" -v target_key="$key" '
    /^\[[^]]+\]/ {
      in_section = ($0 == target_section)
      next
    }
    in_section && $0 ~ "^[[:space:]]*" target_key "[[:space:]]*=" {
      value = substr($0, index($0, "=") + 1)
      sub(/^[[:space:]]*/, "", value)
      sub(/[[:space:]]*$/, "", value)
      if (value ~ /^".*"$/) {
        sub(/^"/, "", value)
        sub(/"$/, "", value)
      }
      print value
      exit
    }
  ' "$file")
  if [ -z "$value" ]; then
    return 1
  fi
  printf '%s' "$value" | sed 's/\\"/"/g; s/\\\\/\\/g'
}

browser_core_from_cores() {
  case "$1" in
    firefox) printf '%s' "firefox" ;;
    all|chromium,firefox|firefox,chromium) printf '%s' "all" ;;
    *) printf '%s' "chromium" ;;
  esac
}

normalize_database_driver() {
  driver=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
  case "$driver" in
    postgres|postgresql) printf '%s' "postgres" ;;
    *) return 1 ;;
  esac
}

normalize_database_driver_or_exit() {
  normalized_database_driver=$(normalize_database_driver "$DATABASE_DRIVER" || true)
  if [ -z "$normalized_database_driver" ]; then
    echo "${RED}Error: unsupported database driver '${DATABASE_DRIVER}'. Use postgres.${NC}"
    exit 1
  fi
  DATABASE_DRIVER="$normalized_database_driver"
}

normalize_container_backend() {
  backend=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
  case "$backend" in
    containerd) printf '%s' "containerd" ;;
    docker) printf '%s' "docker" ;;
    podman) printf '%s' "podman" ;;
    kubernetes|k8s) printf '%s' "kubernetes" ;;
    apple) printf '%s' "apple" ;;
    *) return 1 ;;
  esac
}

normalize_deploy_runtime() {
  runtime=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
  case "$runtime" in
    containerd|docker|podman) printf '%s' "$runtime" ;;
    *) return 1 ;;
  esac
}

validate_version_or_exit() {
  version="$1"
  if [ -z "$version" ] || [ "$version" = "latest" ]; then
    return
  fi
  if printf '%s' "$version" | grep -Eq '^v?[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$'; then
    return
  fi
  echo "${RED}Error: unsupported version '${version}'. Use latest or a semver tag such as v0.8.0.${NC}" >&2
  exit 1
}

normalize_deploy_runtime_or_exit() {
  normalized_deploy_runtime=$(normalize_deploy_runtime "$DEPLOY_RUNTIME" || true)
  if [ -z "$normalized_deploy_runtime" ]; then
    echo "${RED}Error: unsupported deployment runtime '${DEPLOY_RUNTIME}'. Use containerd, docker, or podman.${NC}"
    exit 1
  fi
  DEPLOY_RUNTIME="$normalized_deploy_runtime"
}

normalize_container_backend_or_exit() {
  normalized_container_backend=$(normalize_container_backend "$CONTAINER_BACKEND" || true)
  if [ -z "$normalized_container_backend" ]; then
    echo "${RED}Error: unsupported workspace backend '${CONTAINER_BACKEND}'. Use containerd, docker, podman, kubernetes, or apple.${NC}"
    exit 1
  fi
  CONTAINER_BACKEND="$normalized_container_backend"
}

enforce_compose_container_backend() {
  case "$CONTAINER_BACKEND" in
    containerd|docker|podman)
    return
      ;;
  esac
  if [ "$INSTALL_MODE" = "upgrade" ] && [ "$CONTAINER_BACKEND_SET" = false ]; then
    echo "${YELLOW}ℹ Existing config uses workspace backend '${CONTAINER_BACKEND}'. The one-click compose stack is designed for containerd; reusing your config unchanged.${NC}"
    return
  fi
  echo "${RED}Error: one-click compose installs support workspace backend 'containerd' only.${NC}"
  echo "The server image starts an embedded containerd and mounts the required runtime paths."
  echo "For docker, podman, kubernetes, or apple backends, use a manual deployment and edit [container].backend in config.toml."
  exit 1
}

missing_command() {
  command -v "$1" >/dev/null 2>&1 || printf ' %s' "$1"
}

validate_dry_run_runtime_or_exit() {
  case "$DEPLOY_RUNTIME" in
    containerd)
      RUNTIME_PREREQUISITES="git, containerd, nerdctl, buildkit"
      ;;
    docker)
      RUNTIME_PREREQUISITES="git, docker"
      ;;
    podman)
      RUNTIME_PREREQUISITES="git, podman, podman-compose"
      ;;
  esac
}

print_runtime_plan() {
  echo "${GREEN}Memoh Enterprise install plan${NC}"
  echo "  version: ${MEMOH_DOCKER_VERSION}"
  echo "  image registry: ghcr.io/stringke"
  echo "  deployment runtime: ${DEPLOY_RUNTIME}"
  echo "  runtime prerequisites: ${RUNTIME_PREREQUISITES}"
  echo "  workspace backend: ${CONTAINER_BACKEND}"
  echo "  database backend: postgres"
  echo ""
  echo "  services:"
  echo "    server:              26810"
  echo "    web:                 26811"
  echo "    browser-gateway:     26812"
  echo "    agent-runner:        26813"
  echo "    connector:           26814"
  echo "    integration-gateway: 26815"
  echo "    worker:              26816"
  echo "    postgres:            26817"
  echo "    qdrant-http:         26818"
  echo "    qdrant-grpc:         26819"
  echo "    sparse:              26820"
}

escape_toml_string() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

set_toml_string_value() {
  file="$1"
  section="$2"
  key="$3"
  value=$(escape_toml_string "$4")
  tmp="${file}.tmp.$$"
  if TOML_VALUE="$value" awk -v target_section="[$section]" -v target_key="$key" '
    BEGIN {
      target_value = ENVIRON["TOML_VALUE"]
    }
    /^\[[^]]+\]/ {
      in_section = ($0 == target_section)
    }
    in_section && $0 ~ "^[[:space:]]*" target_key "[[:space:]]*=" {
      indent = $0
      sub(/[^[:space:]].*/, "", indent)
      print indent target_key " = \"" target_value "\""
      next
    }
    { print }
  ' "$file" > "$tmp"; then
    mv "$tmp" "$file"
  else
    rm -f "$tmp"
    return 1
  fi
}

write_env_value() {
  key="$1"
  value=$(printf '%s' "$2" | sed "s/'/\\\\'/g")
  printf "%s='%s'\n" "$key" "$value" >> .env
}

fetch_latest_version() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" 2>/dev/null
  else
    echo "${RED}Error: curl or wget is required${NC}" >&2
    exit 1
  fi
}

sudo_cmd() {
  if [ "$(id -u 2>/dev/null || printf '1')" = "0" ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "${RED}Error: sudo is required to install missing server dependencies.${NC}" >&2
    exit 1
  fi
}

detect_package_manager() {
  if command -v apt-get >/dev/null 2>&1; then
    printf '%s' "apt"
  elif command -v dnf >/dev/null 2>&1; then
    printf '%s' "dnf"
  elif command -v yum >/dev/null 2>&1; then
    printf '%s' "yum"
  else
    return 1
  fi
}

install_packages() {
  packages="$*"
  [ -n "$packages" ] || return 0
  pm=$(detect_package_manager || true)
  case "$pm" in
    apt)
      sudo_cmd apt-get update
      # shellcheck disable=SC2086
      sudo_cmd env DEBIAN_FRONTEND=noninteractive apt-get install -y $packages
      ;;
    dnf)
      # shellcheck disable=SC2086
      sudo_cmd dnf install -y $packages
      ;;
    yum)
      # shellcheck disable=SC2086
      sudo_cmd yum install -y $packages
      ;;
    *)
      echo "${RED}Error: unsupported package manager. Install manually: ${packages}${NC}" >&2
      exit 1
      ;;
  esac
}

ensure_common_tools() {
  missing=""
  for tool in git openssl tar gzip; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      missing="$missing $tool"
    fi
  done
  if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    missing="$missing curl"
  fi
  if [ -n "$missing" ]; then
    echo "${YELLOW}Installing common dependencies:${missing}${NC}"
    # shellcheck disable=SC2086
    install_packages $missing
  fi
}

start_system_service() {
  service="$1"
  if command -v systemctl >/dev/null 2>&1; then
    sudo_cmd systemctl enable --now "$service" >/dev/null 2>&1 || true
  fi
}

install_nerdctl_binary() {
  if command -v nerdctl >/dev/null 2>&1; then
    return
  fi
  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) nerdctl_arch="amd64" ;;
    aarch64|arm64) nerdctl_arch="arm64" ;;
    *)
      echo "${RED}Error: unsupported architecture '${arch}'. Only amd64 and arm64 are supported.${NC}" >&2
      exit 1
      ;;
  esac
  version=$(fetch_latest_nerdctl_version)
  if [ -z "$version" ]; then
    echo "${RED}Error: failed to resolve latest nerdctl release.${NC}" >&2
    exit 1
  fi
  archive="nerdctl-full-${version}-linux-${nerdctl_arch}.tar.gz"
  url="https://github.com/containerd/nerdctl/releases/download/${version}/${archive}"
  tmp="/tmp/${archive}"
  echo "${YELLOW}Installing nerdctl ${version}...${NC}"
  download_file "$url" "$tmp"
  sudo_cmd tar -C /usr/local -xzf "$tmp"
  rm -f "$tmp"
}

fetch_latest_nerdctl_version() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "https://api.github.com/repos/containerd/nerdctl/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
  else
    wget -qO- "https://api.github.com/repos/containerd/nerdctl/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
  fi
}

download_file() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  else
    wget -qO "$out" "$url"
  fi
}

ensure_containerd_runtime() {
  missing=""
  command -v containerd >/dev/null 2>&1 || missing="$missing containerd"
  if [ -n "$missing" ]; then
    echo "${YELLOW}Installing containerd...${NC}"
    # shellcheck disable=SC2086
    install_packages $missing
  fi
  start_system_service containerd
  install_nerdctl_binary
  CONTAINER_CMD="nerdctl"
  if ! nerdctl info >/dev/null 2>&1; then
    if command -v sudo >/dev/null 2>&1 && sudo nerdctl info >/dev/null 2>&1; then
      CONTAINER_CMD="sudo nerdctl"
    else
      echo "${RED}Error: Cannot connect to containerd with nerdctl.${NC}"
      exit 1
    fi
  fi
  if ! $CONTAINER_CMD compose version >/dev/null 2>&1; then
    echo "${RED}Error: nerdctl compose is required.${NC}"
    exit 1
  fi
}

ensure_docker_runtime() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "${YELLOW}Installing Docker Engine...${NC}"
    pm=$(detect_package_manager || true)
    case "$pm" in
      apt) install_packages docker.io docker-compose-plugin ;;
      dnf|yum) install_packages docker docker-compose-plugin ;;
      *) install_packages docker ;;
    esac
  fi
  start_system_service docker
  CONTAINER_CMD="docker"
  if ! docker info >/dev/null 2>&1; then
    if command -v sudo >/dev/null 2>&1 && sudo docker info >/dev/null 2>&1; then
      CONTAINER_CMD="sudo docker"
    else
      echo "${RED}Error: Cannot connect to Docker daemon${NC}"
      echo "Try: sudo usermod -aG docker \$USER && newgrp docker"
      exit 1
    fi
  fi
  if ! $CONTAINER_CMD compose version >/dev/null 2>&1; then
    echo "${RED}Error: Docker Compose v2 is required${NC}"
    exit 1
  fi
}

ensure_podman_runtime() {
  if ! command -v podman >/dev/null 2>&1; then
    echo "${YELLOW}Installing Podman...${NC}"
    install_packages podman podman-compose
  elif ! podman compose version >/dev/null 2>&1 && ! command -v podman-compose >/dev/null 2>&1; then
    echo "${YELLOW}Installing podman-compose...${NC}"
    install_packages podman-compose
  fi
  start_system_service podman.socket
  CONTAINER_CMD="podman"
  if ! podman info >/dev/null 2>&1; then
    if command -v sudo >/dev/null 2>&1 && sudo podman info >/dev/null 2>&1; then
      CONTAINER_CMD="sudo podman"
    else
      echo "${RED}Error: Cannot run podman info.${NC}"
      exit 1
    fi
  fi
  if ! $CONTAINER_CMD compose version >/dev/null 2>&1; then
    echo "${RED}Error: podman compose is required.${NC}"
    exit 1
  fi
}

ensure_deploy_runtime() {
  case "$DEPLOY_RUNTIME" in
    containerd) ensure_containerd_runtime ;;
    docker) ensure_docker_runtime ;;
    podman) ensure_podman_runtime ;;
  esac
  DOCKER="$CONTAINER_CMD"
  echo "${GREEN}✓ Deployment runtime: ${DEPLOY_RUNTIME} (${DOCKER})${NC}"
}

detect_existing_installation() {
  EXISTING_CONFIG_SOURCE=""
  EXISTING_ENV_SOURCE=""
  EXISTING_INSTALL_STATE=false
  EXISTING_DOCKER_STATE=false
  EXISTING_DOCKER_VOLUMES=""
  EXISTING_DOCKER_CONTAINERS=false
  EXISTING_DOCKER_NETWORK=false
  EXISTING_WORKSPACE_FILES=false
  EXISTING_REPO_DIR=false

  if [ -d "$WORKSPACE/$DIR" ]; then
    EXISTING_REPO_DIR=true
    EXISTING_INSTALL_STATE=true
  fi

  if [ -f "$WORKSPACE/config.toml" ]; then
    EXISTING_CONFIG_SOURCE="$WORKSPACE/config.toml"
    EXISTING_WORKSPACE_FILES=true
    EXISTING_INSTALL_STATE=true
    if [ -f "$WORKSPACE/.env" ]; then
      EXISTING_ENV_SOURCE="$WORKSPACE/.env"
    fi
  elif [ -f "$WORKSPACE/$DIR/config.toml" ]; then
    EXISTING_CONFIG_SOURCE="$WORKSPACE/$DIR/config.toml"
    EXISTING_INSTALL_STATE=true
    if [ -f "$WORKSPACE/$DIR/.env" ]; then
      EXISTING_ENV_SOURCE="$WORKSPACE/$DIR/.env"
    fi
  fi

  if [ -f "$WORKSPACE/docker-compose.yml" ] || [ -f "$WORKSPACE/.env" ]; then
    EXISTING_WORKSPACE_FILES=true
    EXISTING_INSTALL_STATE=true
    if [ -z "$EXISTING_ENV_SOURCE" ] && [ -f "$WORKSPACE/.env" ]; then
      EXISTING_ENV_SOURCE="$WORKSPACE/.env"
    fi
  fi

  for volume in $PROJECT_VOLUMES; do
    if $DOCKER volume inspect "$volume" >/dev/null 2>&1; then
      EXISTING_DOCKER_STATE=true
      EXISTING_INSTALL_STATE=true
      EXISTING_DOCKER_VOLUMES="${EXISTING_DOCKER_VOLUMES} ${volume}"
    fi
  done

  for container in $PROJECT_CONTAINERS; do
    if $DOCKER container inspect "$container" >/dev/null 2>&1; then
      EXISTING_DOCKER_STATE=true
      EXISTING_DOCKER_CONTAINERS=true
      EXISTING_INSTALL_STATE=true
      break
    fi
  done

  if $DOCKER network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    EXISTING_DOCKER_STATE=true
    EXISTING_DOCKER_NETWORK=true
    EXISTING_INSTALL_STATE=true
  fi
}

load_existing_settings() {
  if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "admin" "username" || true)
    [ -n "$value" ] && ADMIN_USER="$value"

    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "admin" "password" || true)
    [ -n "$value" ] && ADMIN_PASS="$value"

    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "auth" "jwt_secret" || true)
    [ -n "$value" ] && JWT_SECRET="$value"

    if [ "$DATABASE_DRIVER_SET" = false ]; then
      value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "database" "driver" || true)
      [ -n "$value" ] && DATABASE_DRIVER="$value"
    fi

    if [ "$CONTAINER_BACKEND_SET" = false ]; then
      value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "container" "backend" || true)
      [ -n "$value" ] && CONTAINER_BACKEND="$value"
    fi

    value=$(read_toml_value "$EXISTING_CONFIG_SOURCE" "postgres" "password" || true)
    [ -n "$value" ] && PG_PASS="$value"

  fi

  if [ -n "$EXISTING_ENV_SOURCE" ]; then
    if [ "$USE_SPARSE_SET" = false ]; then
      value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "USE_SPARSE" || true)
      [ -n "$value" ] && USE_SPARSE="$value"
    fi

    if [ "$BROWSER_CORE_SET" = false ]; then
      value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "BROWSER_CORES" || true)
      [ -n "$value" ] && BROWSER_CORE=$(browser_core_from_cores "$value")
    fi

    value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "POSTGRES_PASSWORD" || true)
    [ -n "$value" ] && PG_PASS="$value"

    value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "MEMOH_DATA_DIR" || true)
    [ -n "$value" ] && MEMOH_DATA_DIR="$value"

    if [ "$DATABASE_DRIVER_SET" = false ]; then
      value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "MEMOH_DATABASE_DRIVER" || true)
      [ -n "$value" ] && DATABASE_DRIVER="$value"
    fi

    if [ "$CONTAINER_BACKEND_SET" = false ]; then
      value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "MEMOH_CONTAINER_BACKEND" || true)
      [ -n "$value" ] && CONTAINER_BACKEND="$value"
    fi

    value=$(read_env_file_value "$EXISTING_ENV_SOURCE" "MEMOH_DEPLOY_RUNTIME" || true)
    [ -n "$value" ] && DEPLOY_RUNTIME="$value"
  fi
}

prompt_install_mode() {
  if [ "$SILENT" = true ]; then
    if [ "$INSTALL_MODE" = "auto" ]; then
      if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
        INSTALL_MODE="upgrade"
        echo "${YELLOW}ℹ Existing Memoh installation detected. Reusing existing configuration in silent mode.${NC}"
      elif [ "$EXISTING_DOCKER_STATE" = true ]; then
        echo "${RED}Error: Existing Memoh runtime state was detected but no reusable config.toml was found.${NC}"
        echo "Run again with MEMOH_INSTALL_MODE=reinstall to wipe runtime data, or restore the previous config.toml."
        exit 1
      else
        INSTALL_MODE="fresh"
        if [ "$EXISTING_INSTALL_STATE" = true ]; then
          echo "${YELLOW}ℹ Existing Memoh files were detected, but no runtime state or reusable config.toml was found. Proceeding with a fresh install in silent mode.${NC}"
        fi
      fi
    fi
    return
  fi

  if [ "$INSTALL_MODE" != "auto" ]; then
    return
  fi

  if [ "$EXISTING_INSTALL_STATE" = false ]; then
    INSTALL_MODE="fresh"
    return
  fi

  echo "${YELLOW}Detected existing Memoh installation state:${NC}" > /dev/tty
  if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
    echo "  - Config: ${EXISTING_CONFIG_SOURCE}" > /dev/tty
  fi
  if [ -n "$EXISTING_ENV_SOURCE" ]; then
    echo "  - Env: ${EXISTING_ENV_SOURCE}" > /dev/tty
  fi
  if [ "$EXISTING_REPO_DIR" = true ]; then
    echo "  - Repository checkout: ${WORKSPACE}/${DIR}" > /dev/tty
  fi
  if [ -n "$EXISTING_DOCKER_VOLUMES" ]; then
    echo "  - Runtime volumes:${EXISTING_DOCKER_VOLUMES}" > /dev/tty
  fi
  if [ "$EXISTING_DOCKER_CONTAINERS" = true ]; then
    echo "  - Existing Memoh containers" > /dev/tty
  fi
  if [ "$EXISTING_DOCKER_NETWORK" = true ]; then
    echo "  - Runtime network: ${NETWORK_NAME}" > /dev/tty
  fi
  echo "" > /dev/tty

  if [ -n "$EXISTING_CONFIG_SOURCE" ]; then
    echo "Choose install mode:" > /dev/tty
    echo "  1) Upgrade existing installation (recommended, reuses config and DB password)" > /dev/tty
    echo "  2) Reinstall from scratch (removes Memoh runtime data)" > /dev/tty
    echo "  3) Abort" > /dev/tty
    printf "  Install mode [1]: " > /dev/tty
    read -r input < /dev/tty || true
    case "$input" in
      2) INSTALL_MODE="reinstall" ;;
      3) INSTALL_MODE="abort" ;;
      *) INSTALL_MODE="upgrade" ;;
    esac
  elif [ "$EXISTING_DOCKER_STATE" = true ]; then
    echo "No reusable config.toml was found for a safe upgrade." > /dev/tty
    echo "Choose install mode:" > /dev/tty
    echo "  1) Reinstall from scratch (removes Memoh runtime data)" > /dev/tty
    echo "  2) Abort" > /dev/tty
    printf "  Install mode [2]: " > /dev/tty
    read -r input < /dev/tty || true
    case "$input" in
      1) INSTALL_MODE="reinstall" ;;
      *) INSTALL_MODE="abort" ;;
    esac
  else
    echo "No reusable config.toml or runtime state was found." > /dev/tty
    echo "Choose install mode:" > /dev/tty
    echo "  1) Continue fresh install (recommended)" > /dev/tty
    echo "  2) Abort" > /dev/tty
    printf "  Install mode [1]: " > /dev/tty
    read -r input < /dev/tty || true
    case "$input" in
      2) INSTALL_MODE="abort" ;;
      *) INSTALL_MODE="fresh" ;;
    esac
  fi
}

cleanup_existing_installation() {
  echo "${YELLOW}Removing existing Memoh containers, volumes, and network...${NC}"
  for container in $PROJECT_CONTAINERS; do
    $DOCKER rm -f "$container" >/dev/null 2>&1 || true
  done
  for volume in $PROJECT_VOLUMES; do
    $DOCKER volume rm -f "$volume" >/dev/null 2>&1 || true
  done
  $DOCKER network rm "$NETWORK_NAME" >/dev/null 2>&1 || true
}

show_failure_logs() {
  echo ""
  echo "${RED}Startup failed. Recent database, migration, and server logs:${NC}"
  log_services="postgres migrate server"
  $DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES logs --no-color --tail=200 $log_services || true
}

normalize_deploy_runtime_or_exit
normalize_container_backend_or_exit

if [ "$DRY_RUN" = true ]; then
  if [ -z "$MEMOH_VERSION" ]; then
    MEMOH_VERSION="latest"
  fi
  validate_version_or_exit "$MEMOH_VERSION"
  if [ "$MEMOH_VERSION" = "latest" ]; then
    MEMOH_DOCKER_VERSION="latest"
  else
    MEMOH_DOCKER_VERSION=$(echo "$MEMOH_VERSION" | sed 's/^v//')
  fi
  validate_dry_run_runtime_or_exit
  print_runtime_plan
  exit 0
fi

ensure_common_tools
ensure_deploy_runtime

# Resolve version: use MEMOH_VERSION env if set, otherwise fetch latest release
validate_version_or_exit "$MEMOH_VERSION"
if [ "$VERSION_SET" = true ] && [ "$MEMOH_VERSION" = "latest" ]; then
    echo "${GREEN}✓ Using latest image tag from main checkout${NC}"
elif [ -n "$MEMOH_VERSION" ]; then
    echo "${GREEN}✓ Using specified version: ${MEMOH_VERSION}${NC}"
else
    MEMOH_VERSION=$(fetch_latest_version | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    if [ -n "$MEMOH_VERSION" ]; then
        echo "${GREEN}✓ Latest release: ${MEMOH_VERSION}${NC}"
    else
        echo "${YELLOW}Warning: Failed to fetch latest release tag, falling back to main branch${NC}"
    fi
fi

# Image tag: strip leading "v", fall back to "latest" only when version is unknown
if [ "$MEMOH_VERSION" = "latest" ]; then
    MEMOH_DOCKER_VERSION="latest"
elif [ -n "$MEMOH_VERSION" ]; then
    MEMOH_DOCKER_VERSION=$(echo "$MEMOH_VERSION" | sed 's/^v//')
else
    MEMOH_DOCKER_VERSION="latest"
fi
echo "${GREEN}✓ Image version: ${MEMOH_DOCKER_VERSION}${NC}"

# Generate random JWT secret
gen_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 32
  else
    head -c 32 /dev/urandom | base64 | tr -d '\n'
  fi
}

# Configuration defaults (expand ~ for paths)
WORKSPACE_DEFAULT="${HOME:-/tmp}/memoh"
MEMOH_DATA_DIR_DEFAULT="${HOME:-/tmp}/memoh/data"
ADMIN_USER="admin"
ADMIN_PASS="admin123"
JWT_SECRET="$(gen_secret)"
PG_PASS="memoh123"
WORKSPACE="$WORKSPACE_DEFAULT"
MEMOH_DATA_DIR="$MEMOH_DATA_DIR_DEFAULT"
USE_SPARSE="${USE_SPARSE:-false}"
BROWSER_CORE="${BROWSER_CORE:-chromium}"

if [ "$SILENT" = false ]; then
  echo "Configure Memoh (press Enter to use defaults):" > /dev/tty
  echo "" > /dev/tty

  printf "  Workspace (install and clone here) [%s]: " "~/memoh" > /dev/tty
  read -r input < /dev/tty || true
  if [ -n "$input" ]; then
    case "$input" in
      "~") WORKSPACE="${HOME:-/tmp}" ;;
      "~"/*) WORKSPACE="${HOME:-/tmp}${input#\~}" ;;
      *) WORKSPACE="$input" ;;
    esac
  fi
fi

mkdir -p "$WORKSPACE"
WORKSPACE=$(cd "$WORKSPACE" && pwd)

detect_existing_installation
load_existing_settings
normalize_database_driver_or_exit
normalize_container_backend_or_exit
prompt_install_mode

case "$INSTALL_MODE" in
  auto) INSTALL_MODE="fresh" ;;
  fresh|upgrade|reinstall) ;;
  abort)
    echo "Installation aborted."
    exit 0
    ;;
  *)
    echo "${RED}Error: Unknown install mode '${INSTALL_MODE}'. Use fresh, upgrade, reinstall, or auto.${NC}"
    exit 1
    ;;
esac

if [ "$INSTALL_MODE" = "upgrade" ] && [ -z "$EXISTING_CONFIG_SOURCE" ]; then
  echo "${RED}Error: Upgrade mode requires an existing config.toml to reuse.${NC}"
  exit 1
fi

if [ "$INSTALL_MODE" = "fresh" ] && [ "$EXISTING_DOCKER_STATE" = true ]; then
  echo "${RED}Error: Existing Memoh runtime state was detected. Use upgrade or reinstall instead of fresh.${NC}"
  exit 1
fi
enforce_compose_container_backend

if [ "$SILENT" = false ] && [ "$INSTALL_MODE" != "upgrade" ]; then
  printf "  Data directory (reserved for future bind-mount support) [%s]: " "$MEMOH_DATA_DIR" > /dev/tty
  read -r input < /dev/tty || true
  if [ -n "$input" ]; then
    case "$input" in
      "~") MEMOH_DATA_DIR="${HOME:-/tmp}" ;;
      "~"/*) MEMOH_DATA_DIR="${HOME:-/tmp}${input#\~}" ;;
      *) MEMOH_DATA_DIR="$input" ;;
    esac
  fi

  printf "  Admin username [%s]: " "$ADMIN_USER" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && ADMIN_USER="$input"

  printf "  Admin password [%s]: " "$ADMIN_PASS" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && ADMIN_PASS="$input"

  printf "  JWT secret [current/default value retained]: " > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && JWT_SECRET="$input"

  echo "" > /dev/tty
  echo "  Database backend: PostgreSQL" > /dev/tty
  DATABASE_DRIVER="postgres"
  normalize_database_driver_or_exit

  printf "  Postgres password [%s]: " "$PG_PASS" > /dev/tty
  read -r input < /dev/tty || true
  [ -n "$input" ] && PG_PASS="$input"

  echo "  Workspace backend: containerd (compose default; starts an embedded containerd inside memoh-server)" > /dev/tty
  echo "  Other backends such as docker, kubernetes, and apple are configured manually in config.toml." > /dev/tty

  printf "  Enable sparse memory service? [%s]: " "$( [ "$USE_SPARSE" = true ] && printf 'Y/n' || printf 'y/N' )" > /dev/tty
  read -r input < /dev/tty || true
  case "$input" in
    y|Y|yes|YES) USE_SPARSE=true ;;
    n|N|no|NO) USE_SPARSE=false ;;
  esac

  echo "" > /dev/tty
  echo "  Browser core selection:" > /dev/tty
  echo "    1) Chromium only (default, smaller image)" > /dev/tty
  echo "    2) Firefox only" > /dev/tty
  echo "    3) Both Chromium and Firefox" > /dev/tty
  case "$BROWSER_CORE" in
    firefox) browser_default="2" ;;
    all) browser_default="3" ;;
    *) browser_default="1" ;;
  esac
  printf "  Browser core [%s]: " "$browser_default" > /dev/tty
  read -r input < /dev/tty || true
  case "$input" in
    2) BROWSER_CORE="firefox" ;;
    3) BROWSER_CORE="all" ;;
    "") 
      case "$browser_default" in
        2) BROWSER_CORE="firefox" ;;
        3) BROWSER_CORE="all" ;;
        *) BROWSER_CORE="chromium" ;;
      esac
      ;;
    *) BROWSER_CORE="chromium" ;;
  esac

  echo "" > /dev/tty
elif [ "$INSTALL_MODE" = "upgrade" ]; then
  echo "${GREEN}✓ Upgrade mode: reusing existing configuration and database credentials${NC}"
fi
normalize_database_driver_or_exit
normalize_container_backend_or_exit
enforce_compose_container_backend

# Enter workspace (all operations run here)
cd "$WORKSPACE"

# Clone or update
CLONED_FRESH=false
if [ -d "$DIR" ]; then
    echo "Updating existing installation in $WORKSPACE..."
    cd "$DIR"
    if [ -n "$MEMOH_VERSION" ] && [ "$MEMOH_VERSION" != "latest" ]; then
        git fetch --depth 1 origin tag "$MEMOH_VERSION"
        git checkout "$MEMOH_VERSION"
    else
        git fetch --depth 1 origin main
        git checkout main 2>/dev/null || git checkout -b main --track origin/main
        git reset --hard origin/main
    fi
else
    echo "Cloning Memoh into $WORKSPACE..."
    if [ -n "$MEMOH_VERSION" ] && [ "$MEMOH_VERSION" != "latest" ]; then
        git clone --depth 1 --branch "$MEMOH_VERSION" "$REPO" "$DIR"
    else
        git clone --depth 1 "$REPO" "$DIR"
    fi
    cd "$DIR"
    CLONED_FRESH=true
fi

COMPOSE_FILE_NAME="docker-compose.yml"
if [ ! -f "$COMPOSE_FILE_NAME" ]; then
  echo "${RED}Error: ${COMPOSE_FILE_NAME} is missing in ${MEMOH_VERSION:-the selected checkout}.${NC}"
  echo "Use a newer Memoh version."
  exit 1
fi

# Pin image versions in the selected compose file.
if [ "$MEMOH_DOCKER_VERSION" != "latest" ]; then
    sed -i.bak "s|ghcr.io/stringke/server:latest|ghcr.io/stringke/server:${MEMOH_DOCKER_VERSION}|g" "$COMPOSE_FILE_NAME"
    sed -i.bak "s|ghcr.io/stringke/web:\\${WEB_TAG:-latest}|ghcr.io/stringke/web:\\${WEB_TAG:-${MEMOH_DOCKER_VERSION}}|g" "$COMPOSE_FILE_NAME"
    sed -i.bak "s|ghcr.io/stringke/sparse:latest|ghcr.io/stringke/sparse:${MEMOH_DOCKER_VERSION}|g" "$COMPOSE_FILE_NAME"
    rm -f "${COMPOSE_FILE_NAME}.bak"
    echo "${GREEN}✓ Images pinned to ${MEMOH_DOCKER_VERSION}${NC}"
fi

if [ "$INSTALL_MODE" = "upgrade" ]; then
  if [ "$EXISTING_CONFIG_SOURCE" != "$PWD/config.toml" ]; then
    cp "$EXISTING_CONFIG_SOURCE" ./config.toml
  fi
else
  cp conf/app.docker.toml config.toml
  set_toml_string_value config.toml "admin" "username" "$ADMIN_USER"
  set_toml_string_value config.toml "admin" "password" "$ADMIN_PASS"
  set_toml_string_value config.toml "auth" "jwt_secret" "$JWT_SECRET"
  set_toml_string_value config.toml "database" "driver" "$DATABASE_DRIVER"
  set_toml_string_value config.toml "container" "backend" "$CONTAINER_BACKEND"
  set_toml_string_value config.toml "postgres" "password" "$PG_PASS"
  rm -f config.toml.bak
fi

INSTALL_DIR="$(pwd)"
mkdir -p "$MEMOH_DATA_DIR"
MEMOH_DATA_DIR=$(cd "$MEMOH_DATA_DIR" && pwd)
export MEMOH_CONFIG=./config.toml
export MEMOH_DATA_DIR
export POSTGRES_PASSWORD="${PG_PASS}"

# Resolve browser tag and cores from BROWSER_CORE selection
case "$BROWSER_CORE" in
  firefox)
    BROWSER_TAG_VARIANT="firefox"
    BROWSER_CORES="firefox"
    ;;
  all)
    BROWSER_TAG_VARIANT=""
    BROWSER_CORES="chromium,firefox"
    ;;
  *)
    BROWSER_TAG_VARIANT="chromium"
    BROWSER_CORES="chromium"
    ;;
esac

if [ -n "$BROWSER_TAG_VARIANT" ]; then
  if [ "$MEMOH_DOCKER_VERSION" != "latest" ]; then
    BROWSER_TAG="${MEMOH_DOCKER_VERSION}-${BROWSER_TAG_VARIANT}"
  else
    BROWSER_TAG="${BROWSER_TAG_VARIANT}-latest"
  fi
else
  BROWSER_TAG="${MEMOH_DOCKER_VERSION}"
fi

COMPOSE_FILES="-f ${COMPOSE_FILE_NAME}"
COMPOSE_PROFILES="--profile qdrant --profile browser"
if [ "$USE_SPARSE" = true ]; then
  COMPOSE_PROFILES="$COMPOSE_PROFILES --profile sparse"
  echo "${GREEN}✓ Sparse memory service enabled${NC}"
else
  echo "${YELLOW}ℹ Sparse memory service disabled${NC}"
fi

: > .env
write_env_value "POSTGRES_PASSWORD" "$PG_PASS"
write_env_value "MEMOH_CONFIG" "./config.toml"
write_env_value "MEMOH_DATA_DIR" "$MEMOH_DATA_DIR"
write_env_value "MEMOH_DATABASE_DRIVER" "$DATABASE_DRIVER"
write_env_value "MEMOH_CONTAINER_BACKEND" "$CONTAINER_BACKEND"
write_env_value "MEMOH_DEPLOY_RUNTIME" "$DEPLOY_RUNTIME"
write_env_value "USE_SPARSE" "$USE_SPARSE"
write_env_value "BROWSER_TAG" "$BROWSER_TAG"
write_env_value "BROWSER_CORES" "$BROWSER_CORES"
echo "${GREEN}✓ Database backend: ${DATABASE_DRIVER}${NC}"
echo "${GREEN}✓ Workspace backend: ${CONTAINER_BACKEND}${NC}"
echo "${GREEN}✓ Browser: ${BROWSER_CORE} (image tag: ${BROWSER_TAG})${NC}"

if [ "$INSTALL_MODE" = "reinstall" ]; then
  cleanup_existing_installation
fi

echo ""
echo "${GREEN}Pulling images...${NC}"
$DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES pull

echo ""
echo "${GREEN}Starting services (first startup may take a few minutes)...${NC}"
if ! $DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES up -d; then
  show_failure_logs
  exit 1
fi

# After fresh clone: copy minimal files to workspace and remove clone directory
if [ "$CLONED_FRESH" = true ]; then
  echo ""
  echo "${GREEN}Cleaning up clone directory...${NC}"
  cp "$COMPOSE_FILE_NAME" config.toml .env "$WORKSPACE/"
  mkdir -p "$WORKSPACE/conf"
  cp -r conf/providers "$WORKSPACE/conf/"
  cd "$WORKSPACE"
  rm -rf "$WORKSPACE/$DIR"
  INSTALL_DIR="$WORKSPACE"
  echo "${GREEN}✓ Clone directory removed, minimal install at ${INSTALL_DIR}${NC}"
fi

echo ""
echo "${GREEN}✅ Memoh is running!${NC}"
echo ""
echo "  🔌 API:               http://localhost:26810"
echo "  Web UI:            http://localhost:26811"
echo "  🌍 Browser Gateway:   http://localhost:26812"
echo ""
echo "  🔑 Admin login:       ${ADMIN_USER} / ${ADMIN_PASS}"
echo ""
COMPOSE_CMD="$DOCKER compose $COMPOSE_FILES $COMPOSE_PROFILES"
echo "📋 Commands:"
echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} ps       # Status"
echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} logs -f   # Logs"
echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} down      # Stop"
if [ "$INSTALL_MODE" != "fresh" ]; then
  echo "  cd ${INSTALL_DIR} && ${COMPOSE_CMD} down -v   # Remove containers and runtime data"
fi
echo ""
echo "${YELLOW}⏳ First startup may take 1-2 minutes, please be patient.${NC}"
