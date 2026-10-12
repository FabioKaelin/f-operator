#!/usr/bin/env bash
set -euo pipefail
source /var/lib/minikube/controlplane/ssd.env
[[ $(id -u) == 0 ]]
[[ $(findmnt -n -o UUID --target "$ETCD_SSD_MEDIA") == "$ETCD_SSD_UUID" ]] || { echo 'Expected SSD not mounted; refusing etcd startup.' >&2; exit 1; }
[[ -f "$ETCD_SSD_IMAGE" && -f "$ETCD_SSD_KEY" ]]
[[ $(stat -c '%a:%u' "$ETCD_SSD_KEY") == 600:0 ]]
cryptsetup isLuks "$ETCD_SSD_IMAGE"
if ! cryptsetup status "$ETCD_SSD_MAPPING" >/dev/null 2>&1; then
 loop=$(losetup -f)
 [[ -b "$loop" ]] || mknod "$loop" b 7 "${loop##*/loop}"
 cryptsetup open --key-file "$ETCD_SSD_KEY" "$ETCD_SSD_IMAGE" "$ETCD_SSD_MAPPING"
fi
backing=$(cryptsetup status "$ETCD_SSD_MAPPING" | awk '$1=="device:"{print $2}')
[[ $(losetup -n -O BACK-FILE "$backing") == "$ETCD_SSD_IMAGE" ]]
device=/dev/mapper/$ETCD_SSD_MAPPING
[[ -b "$device" ]] || dmsetup mknodes "$ETCD_SSD_MAPPING"
[[ $(blkid -s LABEL -o value "$device") == fabkli-etcd ]]
mkdir -p "$ETCD_SSD_TARGET"
if ! mountpoint -q "$ETCD_SSD_TARGET"; then
 mount -o noatime,nodev,nosuid,noexec "$device" "$ETCD_SSD_TARGET"
fi
[[ $(findmnt -n -o FSTYPE --target "$ETCD_SSD_TARGET") == ext4 ]]
[[ $(readlink -f "$(findmnt -n -o SOURCE --target "$ETCD_SSD_TARGET")") == $(readlink -f "$device") ]]
chmod 0700 "$ETCD_SSD_TARGET"

# A recreated Minikube filesystem must never silently use the stale SD member.
python3 - <<'PYCODE'
import json,pathlib
journal=pathlib.Path('/var/lib/minikube/controlplane/migration.json')
if journal.exists() and json.loads(journal.read_text()).get('phase') in ('manifest-configured','kubelet-started'):
    manifest=pathlib.Path('/etc/kubernetes/manifests/etcd.yaml').read_text()
    if '      path: /var/lib/minikube/etcd-ssd/data\n' not in manifest:
        raise SystemExit('Migrated etcd manifest was replaced; refuse stale SD startup and restore the SSD hostPath first.')
PYCODE
