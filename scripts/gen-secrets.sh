#!/usr/bin/env bash
# Render sops-encrypted secrets into the gitignored runtime env file the metrics
# stack reads. Run on a host that holds the age private key (the dev host); then
# scp deploy/metrics/.env to the deploy target alongside docker-compose.yml.
set -euo pipefail
cd "$(dirname "$0")/.."
export SOPS_AGE_KEY_FILE="${SOPS_AGE_KEY_FILE:-$HOME/.config/sops/age/keys.txt}"

OUT="deploy/metrics/.env"
PW="$(sops -d secrets/metrics.yaml | awk -F': ' '/^grafana_admin_password:/{print $2}')"
[ -n "$PW" ] || { echo "failed to decrypt grafana_admin_password" >&2; exit 1; }

umask 077
printf 'GRAFANA_ADMIN_PASSWORD=%s\n' "$PW" > "$OUT"
echo "wrote $OUT (gitignored)"
