#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$ROOT_DIR/bin"
VENV_DIR="$ROOT_DIR/.venv"
USER_BIN_DIR="${HOME}/.local/bin"
SYSTEMD_USER_DIR="${HOME}/.config/systemd/user"
CONFIG_DIR="${HOME}/.config/voxi"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
VOXI_BIN="${USER_BIN_DIR}/voxi"
SERVICE_FILE="${SYSTEMD_USER_DIR}/voxi.service"
WORKER_PYTHON_BIN="${VENV_DIR}/bin/python"
NVIDIA_REBOOT_REQUIRED=0

log() {
  printf '[voxi-setup] %s\n' "$*"
}

warn() {
  printf '[voxi-setup] WARNING: %s\n' "$*" >&2
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    warn "required command missing: $1"
    return 1
  fi
}

config_has_key() {
  local key="$1"
  local file="$2"
  grep -Eq "^${key}:" "$file"
}

has_ollama_systemd_unit() {
  systemctl list-unit-files --type=service --no-legend ollama.service 2>/dev/null | grep -Eq '^ollama\.service([[:space:]]|$)'
}

has_nvidia_hardware() {
  lspci | grep -qi 'nvidia'
}

dnf_package_installed() {
  rpm -q "$1" >/dev/null 2>&1
}

dnf_package_available() {
  dnf -q list --available "$1" >/dev/null 2>&1
}

ensure_rpmfusion_nvidia_repo() {
  if dnf_package_available akmod-nvidia || dnf_package_installed akmod-nvidia; then
    return 0
  fi

  local fedora_release
  fedora_release="$(rpm -E %fedora)"

  log "enabling RPM Fusion repositories required for NVIDIA packages"
  sudo dnf install -y \
    "https://download1.rpmfusion.org/free/fedora/rpmfusion-free-release-${fedora_release}.noarch.rpm" \
    "https://download1.rpmfusion.org/nonfree/fedora/rpmfusion-nonfree-release-${fedora_release}.noarch.rpm"
}

is_fedora() {
  [[ -f /etc/os-release ]] || return 1
  # shellcheck disable=SC1091
  source /etc/os-release
  [[ "${ID:-}" == "fedora" ]]
}

ensure_directory() {
  mkdir -p "$1"
}

install_dnf_packages() {
  if ! is_fedora; then
    warn "setup.sh is Fedora-oriented. Detected a different distribution; package install steps are skipped."
    return 0
  fi

  require_command sudo
  require_command dnf

  local -a packages=()
  command -v go >/dev/null 2>&1 || packages+=(golang)
  command -v python3 >/dev/null 2>&1 || packages+=(python3)
  command -v pip3 >/dev/null 2>&1 || packages+=(python3-pip)
  command -v pw-record >/dev/null 2>&1 || packages+=(pipewire-utils)
  command -v wtype >/dev/null 2>&1 || packages+=(wtype)
  command -v wl-copy >/dev/null 2>&1 || packages+=(wl-clipboard)
  command -v notify-send >/dev/null 2>&1 || packages+=(libnotify)
  command -v curl >/dev/null 2>&1 || packages+=(curl)
  command -v git >/dev/null 2>&1 || packages+=(git)
  command -v lspci >/dev/null 2>&1 || packages+=(pciutils)

  if ((${#packages[@]} == 0)); then
    log "system packages already satisfied"
    return 0
  fi

  log "installing Fedora packages: ${packages[*]}"
  sudo dnf install -y "${packages[@]}"
}

ensure_nvidia_driver_stack() {
  if ! is_fedora; then
    return 0
  fi
  if ! command -v lspci >/dev/null 2>&1 || ! has_nvidia_hardware; then
    log "no NVIDIA hardware detected; skipping NVIDIA driver setup"
    return 0
  fi

  require_command sudo
  require_command dnf

  ensure_rpmfusion_nvidia_repo

  local -a packages=()
  local needs_akmods=0
  dnf_package_installed akmod-nvidia || packages+=(akmod-nvidia)
  dnf_package_installed xorg-x11-drv-nvidia-cuda || packages+=(xorg-x11-drv-nvidia-cuda)
  dnf_package_installed "kernel-devel-$(uname -r)" || packages+=("kernel-devel-$(uname -r)")

  if ((${#packages[@]} > 0)); then
    log "installing NVIDIA driver packages: ${packages[*]}"
    sudo dnf install -y "${packages[@]}"
    needs_akmods=1
  else
    log "NVIDIA driver packages already installed"
  fi

  if ! modinfo nvidia >/dev/null 2>&1; then
    needs_akmods=1
  fi

  if ((needs_akmods == 1)); then
    if command -v akmods >/dev/null 2>&1; then
      log "building NVIDIA kernel modules for $(uname -r)"
      if ! sudo akmods --force --kernels "$(uname -r)"; then
        warn "akmods build failed. Re-run: sudo akmods --force --kernels \"$(uname -r)\""
        NVIDIA_REBOOT_REQUIRED=1
        return 0
      fi
    else
      warn "akmods command not found; NVIDIA modules may not be ready until after reboot"
      NVIDIA_REBOOT_REQUIRED=1
    fi
  else
    log "NVIDIA kernel modules already built; skipping akmods rebuild"
  fi

  if command -v nvidia-smi >/dev/null 2>&1 && nvidia-smi >/dev/null 2>&1; then
    log "nvidia-smi is operational"
    return 0
  fi

  NVIDIA_REBOOT_REQUIRED=1
  warn "NVIDIA packages are installed but runtime is not active yet. Reboot and rerun ./scripts/setup.sh"
}

ensure_ollama() {
  if command -v ollama >/dev/null 2>&1; then
    log "ollama already installed"
  elif is_fedora; then
    require_command sudo
    require_command dnf
    log "installing Ollama from Fedora repositories"
    sudo dnf install -y ollama
  else
    require_command curl
    log "installing Ollama via upstream installer"
    curl -fsSL https://ollama.com/install.sh | sh
  fi

  if command -v systemctl >/dev/null 2>&1 && has_ollama_systemd_unit; then
    if systemctl is-enabled --quiet ollama.service && systemctl is-active --quiet ollama.service; then
      log "ollama.service already enabled and running"
    else
      log "enabling ollama.service"
      sudo systemctl enable --now ollama.service
    fi
  else
    warn "No system Ollama service detected. Start it manually with 'ollama serve' before using Voxi."
  fi
}

ensure_python_env() {
  require_command python3

  if [[ ! -x "${VENV_DIR}/bin/python" ]]; then
    log "creating Python virtual environment"
    if ! python3 -m venv "${VENV_DIR}"; then
      warn "python3 -m venv failed; falling back to user-site install"
      python3 -m pip install --user --break-system-packages -e "${ROOT_DIR}/worker" -r "${ROOT_DIR}/worker/requirements-dev.txt"
      WORKER_PYTHON_BIN="$(command -v python3)"
      return 0
    fi
  else
    log "reusing Python virtual environment at ${VENV_DIR}"
  fi

  log "installing worker package and test tooling"
  "${VENV_DIR}/bin/pip" install --upgrade pip setuptools wheel
  "${VENV_DIR}/bin/pip" install -e "${ROOT_DIR}/worker" -r "${ROOT_DIR}/worker/requirements-dev.txt"
  WORKER_PYTHON_BIN="${VENV_DIR}/bin/python"
}

build_binary() {
  require_command go
  ensure_directory "${BIN_DIR}"
  ensure_directory "${USER_BIN_DIR}"

  log "building Voxi binary"
  (cd "${ROOT_DIR}" && go build -o "${BIN_DIR}/voxi" ./cmd/voxi)
  install -m 0755 "${BIN_DIR}/voxi" "${VOXI_BIN}"
}

ensure_config() {
  ensure_directory "${CONFIG_DIR}"

  if [[ ! -f "${CONFIG_FILE}" ]]; then
    log "creating default config at ${CONFIG_FILE}"
    cat > "${CONFIG_FILE}" <<EOF
hotkey_command: "voxi toggle"
asr_model: "nvidia/parakeet-tdt-0.6b-v2"
llm_runtime: "ollama"
llm_model: "gemma3:4b"
insert_method: "wtype"
notification_timeout_ms: 2200
asr_timeout_ms: 1500
llm_timeout_ms: 1200
insertion_timeout_ms: 200
worker_python: "${WORKER_PYTHON_BIN}"
worker_entrypoint: "voxi_worker"
ollama_url: "http://127.0.0.1:11434"
EOF
    return 0
  fi

  if ! config_has_key "worker_python" "${CONFIG_FILE}"; then
    log "adding worker_python to existing config"
    printf '\nworker_python: "%s"\n' "${WORKER_PYTHON_BIN}" >> "${CONFIG_FILE}"
  fi

  if ! config_has_key "worker_entrypoint" "${CONFIG_FILE}"; then
    log "adding worker_entrypoint to existing config"
    printf '\nworker_entrypoint: "voxi_worker"\n' >> "${CONFIG_FILE}"
  fi
}

install_systemd_service() {
  ensure_directory "${SYSTEMD_USER_DIR}"
  install -m 0644 "${ROOT_DIR}/systemd/voxi.service" "${SERVICE_FILE}"

  if command -v systemctl >/dev/null 2>&1; then
    if systemctl --user daemon-reload >/dev/null 2>&1; then
      log "reloaded systemd user manager"
      systemctl --user enable voxi.service >/dev/null 2>&1 || warn "could not enable voxi.service automatically"
    else
      warn "systemctl --user is unavailable in this session; enable the service later with: systemctl --user enable --now voxi.service"
    fi
  fi
}

check_nvidia_prereqs() {
  if command -v nvidia-smi >/dev/null 2>&1; then
    log "nvidia-smi detected; Voxi can attempt CUDA acceleration"
    return 0
  fi

  if command -v lspci >/dev/null 2>&1 && has_nvidia_hardware; then
    warn "NVIDIA hardware detected but nvidia-smi is unavailable. Install and validate the Fedora NVIDIA driver stack before expecting CUDA acceleration."
  else
    log "no NVIDIA runtime detected; CPU fallback is expected"
  fi
}

run_doctor() {
  log "running voxi doctor"
  if ! "${VOXI_BIN}" doctor; then
    warn "voxi doctor reported readiness issues. Review the output above before relying on the daemon."
    return 1
  fi
}

main() {
  ensure_directory "${USER_BIN_DIR}"
  install_dnf_packages
  ensure_nvidia_driver_stack
  ensure_ollama
  ensure_python_env
  build_binary
  ensure_config
  install_systemd_service
  check_nvidia_prereqs
  run_doctor

  if [[ "${NVIDIA_REBOOT_REQUIRED}" -eq 1 ]]; then
    warn "NVIDIA setup needs a reboot before GPU acceleration is available."
    warn "After reboot, rerun ./scripts/setup.sh once to complete readiness checks."
  fi

  log "setup complete"
  log "Add ${USER_BIN_DIR} to PATH if it is not already present."
  log "Configure a GNOME custom shortcut: Super+I -> voxi toggle"
}

main "$@"
