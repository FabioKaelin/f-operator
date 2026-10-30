#!/bin/bash
set -euo pipefail
backup=/var/backups/k3s-preparation-20261029
install -d -m 700 "$backup"
# Existing production tools/configuration must not be replaced.
test ! -e /etc/rancher/k3s/config.yaml
test ! -e /etc/systemd/system/k3s.service
curl --fail --location --retry 3 https://get.k3s.io -o "$backup/install-k3s.sh"
INSTALL_K3S_VERSION='v1.35.9+k3s1' INSTALL_K3S_SKIP_START=true INSTALL_K3S_SKIP_ENABLE=true INSTALL_K3S_SYMLINK=skip sh "$backup/install-k3s.sh" server
install -d -m 700 /etc/rancher/k3s
cat > /etc/rancher/k3s/config.yaml <<'CONF'
node-name: raspy5
node-ip: 192.168.1.133
advertise-address: 192.168.1.133
bind-address: 192.168.1.133
flannel-iface: eth0
tls-san:
  - 192.168.1.133
data-dir: /var/lib/rancher/k3s
write-kubeconfig-mode: "0600"
secrets-encryption: true
disable:
  - traefik
  - servicelb
node-label:
  - fabkli.ch/node=pi5
  - fabkli.ch/workload-tier=essential
  - fabkli.ch/production=true
cluster-cidr: 10.42.0.0/16
service-cidr: 10.43.0.0/16
kube-proxy-arg:
  - nodeport-addresses=192.168.1.0/24
kubelet-arg:
  - system-reserved=cpu=250m,memory=512Mi
  - kube-reserved=cpu=250m,memory=256Mi
  - eviction-hard=memory.available<256Mi,nodefs.available<15%,imagefs.available<15%,nodefs.inodesFree<5%
  - container-log-max-size=10Mi
  - container-log-max-files=3
CONF
chmod 600 /etc/rancher/k3s/config.yaml
install -d -m 755 /etc/systemd/system/k3s.service.d
cat > /etc/systemd/system/k3s.service.d/10-cutover-gate.conf <<'UNIT'
[Unit]
# Created only during coordinated cutover after backups, swap and conflict checks.
ConditionPathExists=/etc/rancher/k3s/cutover-approved
UNIT
systemctl daemon-reload
install -d -m 700 /var/lib/fabkli/database-restore
k3s --version
systemctl is-enabled k3s || true
