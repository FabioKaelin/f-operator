#!/usr/bin/env bash
set -euo pipefail
source /var/lib/minikube/controlplane/ssd.env
[[ $(id -u) == 0 ]]
[[ $(findmnt -n -o UUID --target "$ETCD_SSD_MEDIA") == "$ETCD_SSD_UUID" ]]
[[ $(df -Pm "$ETCD_SSD_MEDIA" | awk 'NR==2{print $4}') -ge 8192 ]]
command -v cryptsetup >/dev/null
umask 077
mkdir -p /var/lib/minikube/controlplane "$(dirname "$ETCD_SSD_IMAGE")"
chmod 0700 /var/lib/minikube/controlplane
if [[ ! -f "$ETCD_SSD_KEY" ]]; then
 [[ ! -e "$ETCD_SSD_IMAGE" ]] || { echo 'Existing encrypted image has no key; refusing overwrite.' >&2; exit 1; }
 dd if=/dev/urandom of="$ETCD_SSD_KEY" bs=64 count=1 status=none
 chmod 0600 "$ETCD_SSD_KEY"
fi
if [[ ! -e "$ETCD_SSD_IMAGE" ]]; then
 (set -o noclobber; : > "$ETCD_SSD_IMAGE")
 truncate -s 4G "$ETCD_SSD_IMAGE"
 # Low-memory KDF; the key is 512 bits of random material, not a password.
 cryptsetup luksFormat --batch-mode --type luks2 --pbkdf pbkdf2 --key-file "$ETCD_SSD_KEY" "$ETCD_SSD_IMAGE"
 loop=$(losetup -f)
 [[ -b "$loop" ]] || mknod "$loop" b 7 "${loop##*/loop}"
 cryptsetup open --key-file "$ETCD_SSD_KEY" "$ETCD_SSD_IMAGE" "$ETCD_SSD_MAPPING"
 mkfs.ext4 -q -m 1 -L fabkli-etcd "/dev/mapper/$ETCD_SSD_MAPPING"
fi
/var/lib/minikube/controlplane/mount-etcd-ssd.sh
python3 /var/lib/minikube/controlplane/fsync-probe.py "$ETCD_SSD_TARGET"
echo 'Encrypted SSD filesystem prepared; live etcd unchanged.'
