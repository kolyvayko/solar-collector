#!/usr/bin/env bash
# Nightly backup of VictoriaMetrics data to a remote host over SSH.
# Strategy: create a consistent VM snapshot (hardlinks, point-in-time), stream it
# as a gzip tarball to the backup host over SSH, prune to the newest $KEEP, drop
# the snapshot. Runs as the service user: needs passwordless sudo (snapshot files
# are root-owned) and an SSH key to the backup host. Adjust the vars below.
set -euo pipefail

VM_URL="http://localhost:8428"
VMDATA="/home/solar/solar-metrics/vmdata"
NAS="backup-user@backup-host"
NAS_DIR="backups/solar-metrics"          # relative to the NAS user's home
KEEP=30
STAMP="$(date +%Y%m%d-%H%M%S)"

# 1. consistent snapshot via VM API
SNAP="$(curl -fsS "$VM_URL/snapshot/create" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["snapshot"])')"
[ -n "$SNAP" ] || { echo "snapshot create failed" >&2; exit 1; }
trap 'curl -fsS "$VM_URL/snapshot/delete?snapshot=$SNAP" >/dev/null 2>&1 || true' EXIT

# 2. stream snapshot -> NAS (sudo tar to read root-owned snapshot files)
sudo tar -C "$VMDATA/snapshots" -czf - "$SNAP" \
  | ssh -o BatchMode=yes "$NAS" "cat > '$NAS_DIR/solar-vmdata-$STAMP.tar.gz'"

# 3. retention: keep the newest $KEEP tarballs on the NAS
ssh -o BatchMode=yes "$NAS" \
  "ls -1t '$NAS_DIR'/solar-vmdata-*.tar.gz 2>/dev/null | tail -n +$((KEEP+1)) | xargs -r rm -f"

echo "backup ok: solar-vmdata-$STAMP.tar.gz (snapshot $SNAP)"
