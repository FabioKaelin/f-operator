#!/bin/bash
set -euo pipefail
backup=/var/backups/k3s-preparation-20261029
install -d -m 700 "$backup"
cp -an /boot/firmware/cmdline.txt /etc/fstab /etc/rpi/swap.conf "$backup/"
python3 - <<'PY'
from pathlib import Path
p=Path('/boot/firmware/cmdline.txt')
s=p.read_text().strip().split()
s=[x for x in s if x not in ['cgroup_disable=memory','cgroup_memory=0','cgroup_enable=memory','cgroup_memory=1']]
s+=['cgroup_memory=1','cgroup_enable=memory']
p.write_text(' '.join(s)+'\n')
PY
# Disable Raspberry Pi's generated swap persistently; keep backup data intact.
mkdir -p /etc/rpi/swap.conf.d
printf '[Main]\nMechanism=none\n' > /etc/rpi/swap.conf.d/90-k3s.conf
swapoff -a
cat > /etc/modules-load.d/k3s.conf <<'MOD'
overlay
br_netfilter
MOD
modprobe overlay
modprobe br_netfilter
cat > /etc/sysctl.d/90-k3s.conf <<'SYS'
net.ipv4.ip_forward = 1
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
SYS
sysctl -p /etc/sysctl.d/90-k3s.conf
apt-get update -qq
DEBIAN_FRONTEND=noninteractive apt-get install -y curl ca-certificates iptables conntrack nfs-common socat ripgrep
install -d -m 700 /etc/rancher/k3s /var/lib/rancher/k3s
curl --fail --location --retry 3 https://get.k3s.io -o "$backup/install-k3s.sh"
INSTALL_K3S_VERSION='v1.35.9+k3s1' INSTALL_K3S_SKIP_START=true INSTALL_K3S_SKIP_ENABLE=true sh "$backup/install-k3s.sh" agent
cat > /etc/rancher/k3s/config.yaml <<'CONF'
node-name: raspy4
node-label:
  - fabkli.ch/node=pi4
  - fabkli.ch/workload-tier=optional
kube-proxy-arg:
  - nodeport-addresses=192.168.1.0/24
kubelet-arg:
  - system-reserved=cpu=250m,memory=512Mi,ephemeral-storage=20Gi
  - kube-reserved=cpu=250m,memory=256Mi,ephemeral-storage=2Gi
  - eviction-hard=memory.available<256Mi,nodefs.available<15%,imagefs.available<15%,nodefs.inodesFree<5%
  - container-log-max-size=10Mi
  - container-log-max-files=3
CONF
chmod 600 /etc/rancher/k3s/config.yaml
# Joining requires a server endpoint and token file; deliberately not invented here.
install -d -m 755 /srv/sabnzbd/incomplete /srv/sabnzbd/complete
chown -R 1000:1000 /srv/sabnzbd
k3s --version
systemctl is-enabled k3s-agent || true
