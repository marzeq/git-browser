#!/bin/sh
set -eu

runtime_dir=/run/git-browser-appliance
for service in sshd git-browser; do
  pid_file="${runtime_dir}/${service}.pid"
  test -s "${pid_file}"
  kill -0 "$(cat "${pid_file}")" 2>/dev/null
done

listen=${GIT_BROWSER_LISTEN:-0.0.0.0:8080}
port=${listen##*:}
case "${port}" in
  ''|*[!0-9]*) exit 1 ;;
esac

curl --fail --silent --show-error --max-time 3 --noproxy '*' \
  "http://127.0.0.1:${port}/" >/dev/null
