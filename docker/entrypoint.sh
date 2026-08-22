#!/usr/bin/env bash
set -Eeuo pipefail

readonly GIT_USER=git
readonly GIT_GROUP=git
readonly STATE_DIR="${GITOLITE_HOME:-/var/lib/gitolite}"
readonly REPOSITORIES_DIR="${STATE_DIR}/repositories"
readonly HOST_KEY_DIR="${STATE_DIR}/.ssh-host-keys"
readonly RUNTIME_DIR=/run/git-browser-appliance
readonly ADMIN_KEY_FILE="${GITOLITE_ADMIN_KEY_FILE:-/run/secrets/gitolite_admin_key}"
readonly ADMIN_NAME="${GITOLITE_ADMIN_NAME:-admin}"

log() {
  printf '[git-browser] %s\n' "$*" >&2
}

die() {
  log "error: $*"
  exit 1
}

run_as_git() {
  runuser -u "${GIT_USER}" -- env \
    HOME="${STATE_DIR}" \
    USER="${GIT_USER}" \
    LOGNAME="${GIT_USER}" \
    PATH="${STATE_DIR}/bin:/usr/local/bin:/usr/bin:/bin" \
    "$@"
}

prepare_state_directory() {
  install -d -o "${GIT_USER}" -g "${GIT_GROUP}" -m 0750 "${STATE_DIR}"

  case "${GITOLITE_FIX_PERMISSIONS:-false}" in
    1|true|TRUE|yes|YES)
      log "repairing ownership under ${STATE_DIR}"
      find "${STATE_DIR}" -xdev -path "${HOST_KEY_DIR}" -prune -o \
        -exec chown "${GIT_USER}:${GIT_GROUP}" {} +
      ;;
    0|false|FALSE|no|NO|'') ;;
    *) die "GITOLITE_FIX_PERMISSIONS must be true or false" ;;
  esac

  run_as_git test -w "${STATE_DIR}" || die \
    "${STATE_DIR} is not writable by uid 1000; fix the volume ownership or start once with GITOLITE_FIX_PERMISSIONS=true"

  run_as_git mkdir -p "${STATE_DIR}/bin" "${STATE_DIR}/.ssh"
  chmod 0700 "${STATE_DIR}/.ssh"
  touch "${STATE_DIR}/.ssh/authorized_keys"
  chown "${GIT_USER}:${GIT_GROUP}" "${STATE_DIR}/.ssh/authorized_keys"
  chmod 0600 "${STATE_DIR}/.ssh/authorized_keys"
}

install_gitolite() {
  log "installing Gitolite into persistent state"
  run_as_git /opt/gitolite/install -to "${STATE_DIR}/bin"
}

is_initialized() {
  [[ -d "${REPOSITORIES_DIR}/gitolite-admin.git" && -f "${STATE_DIR}/.gitolite/conf/gitolite.conf" ]]
}

has_partial_state() {
  [[ -e "${REPOSITORIES_DIR}/gitolite-admin.git" || -e "${STATE_DIR}/.gitolite" || -e "${STATE_DIR}/.gitolite.rc" ]]
}

validate_admin_key() {
  [[ "${ADMIN_NAME}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || die \
    "GITOLITE_ADMIN_NAME must contain only letters, digits, dot, underscore, and hyphen"
  [[ -f "${ADMIN_KEY_FILE}" && -r "${ADMIN_KEY_FILE}" ]] || die \
    "first boot requires a readable public key at ${ADMIN_KEY_FILE}"
  grep -Eq '^(ssh-(ed25519|rsa)|ecdsa-sha2-nistp(256|384|521)|sk-(ssh-ed25519|ecdsa-sha2-nistp256)@openssh\.com)[[:space:]]' \
    "${ADMIN_KEY_FILE}" || die "${ADMIN_KEY_FILE} does not look like an OpenSSH public key"
  ssh-keygen -l -f "${ADMIN_KEY_FILE}" >/dev/null 2>&1 || die \
    "${ADMIN_KEY_FILE} is not a valid OpenSSH public key"
}

initialize_gitolite() {
  validate_admin_key

  local setup_dir setup_key
  setup_dir="$(mktemp -d /run/gitolite-setup.XXXXXX)"
  setup_key="${setup_dir}/${ADMIN_NAME}.pub"
  install -o "${GIT_USER}" -g "${GIT_GROUP}" -m 0600 "${ADMIN_KEY_FILE}" "${setup_key}"

  log "initializing Gitolite administrator ${ADMIN_NAME}"
  if ! run_as_git "${STATE_DIR}/bin/gitolite" setup -pk "${setup_key}"; then
    rm -rf -- "${setup_dir}"
    die "Gitolite setup failed"
  fi
  rm -rf -- "${setup_dir}"
}

configure_gitolite() {
  if is_initialized; then
    log "existing Gitolite state found; refreshing generated files and hooks"
    run_as_git "${STATE_DIR}/bin/gitolite" setup
    return
  fi
  if has_partial_state; then
    die "partial Gitolite state found in ${STATE_DIR}; restore a complete volume or clear it intentionally"
  fi
  initialize_gitolite
}

prepare_ssh() {
  install -d -o root -g root -m 0700 "${HOST_KEY_DIR}"
  if [[ ! -s "${HOST_KEY_DIR}/ssh_host_ed25519_key" ]]; then
    log "generating persistent ED25519 SSH host key"
    ssh-keygen -q -t ed25519 -N '' -f "${HOST_KEY_DIR}/ssh_host_ed25519_key"
  fi
  if [[ ! -s "${HOST_KEY_DIR}/ssh_host_rsa_key" ]]; then
    log "generating persistent RSA SSH host key"
    ssh-keygen -q -t rsa -b 3072 -N '' -f "${HOST_KEY_DIR}/ssh_host_rsa_key"
  fi
  chown -R root:root "${HOST_KEY_DIR}"
  chmod 0700 "${HOST_KEY_DIR}"
  chmod 0600 "${HOST_KEY_DIR}"/ssh_host_*_key
  chmod 0644 "${HOST_KEY_DIR}"/ssh_host_*_key.pub

  install -d -m 0755 /run/sshd "${RUNTIME_DIR}"
  /usr/sbin/sshd -t -f /etc/ssh/sshd_config
}

start_services() {
  local -a browser_args
  browser_args=(
    -root "${REPOSITORIES_DIR}"
    -listen "${GIT_BROWSER_LISTEN:-0.0.0.0:8080}"
  )
  if [[ -n "${GIT_BROWSER_CLONE_SSH_PREFIX:-}" ]]; then
    browser_args+=(-clone-ssh-prefix "${GIT_BROWSER_CLONE_SSH_PREFIX}")
  fi
  if [[ -n "${GIT_BROWSER_CLONE_HTTPS_PREFIX:-}" ]]; then
    browser_args+=(-clone-https-prefix "${GIT_BROWSER_CLONE_HTTPS_PREFIX}")
  fi
  if [[ -n "${GIT_BROWSER_HIDE:-}" ]]; then
    browser_args+=(-hide "${GIT_BROWSER_HIDE}")
  fi

  log "starting SSH on :22"
  /usr/sbin/sshd -D -e -f /etc/ssh/sshd_config &
  sshd_pid=$!
  printf '%s\n' "${sshd_pid}" >"${RUNTIME_DIR}/sshd.pid"

  log "starting git-browser on ${GIT_BROWSER_LISTEN:-0.0.0.0:8080}"
  run_as_git /usr/local/bin/git-browser "${browser_args[@]}" &
  browser_pid=$!
  printf '%s\n' "${browser_pid}" >"${RUNTIME_DIR}/git-browser.pid"
}

stop_services() {
  local signal="${1:-TERM}"
  trap - TERM INT
  log "stopping services"
  kill -s "${signal}" "${browser_pid:-}" "${sshd_pid:-}" 2>/dev/null || true
  wait "${browser_pid:-}" 2>/dev/null || true
  wait "${sshd_pid:-}" 2>/dev/null || true
}

supervise_services() {
  local status=0 stopped=
  trap 'stop_services TERM; exit 0' TERM INT

  set +e
  wait -n "${sshd_pid}" "${browser_pid}"
  status=$?
  set -e

  if ! kill -0 "${sshd_pid}" 2>/dev/null; then
    stopped=sshd
  elif ! kill -0 "${browser_pid}" 2>/dev/null; then
    stopped=git-browser
  else
    stopped='a service'
  fi
  log "${stopped} exited unexpectedly with status ${status}"
  stop_services TERM
  exit "${status}"
}

main() {
  [[ "${STATE_DIR}" = /var/lib/gitolite ]] || die \
    "GITOLITE_HOME is fixed at /var/lib/gitolite in this image"
  prepare_state_directory
  install_gitolite
  configure_gitolite
  prepare_ssh
  start_services
  supervise_services
}

main "$@"
