#!/bin/bash
set -euo pipefail
install -d -m 755 /etc/systemd/system/k3s-agent.service.d
cat > /usr/local/sbin/k3s-worker-preflight <<'CHECK'
#!/bin/bash
set -euo pipefail
grep -qw memory /sys/fs/cgroup/cgroup.controllers
if [ -n "$(swapon --noheadings --show=NAME)" ]; then echo 'k3s requires swap disabled' >&2; exit 1; fi
if [ "$(cat /sys/class/net/eth0/carrier 2>/dev/null || true)" != 1 ]; then echo 'Connect wired Ethernet before joining k3s' >&2; exit 1; fi
ip -4 -o addr show dev eth0 scope global | grep -q 'inet '
test -s /etc/rancher/k3s/agent-token
test "$(stat -c %u /etc/rancher/k3s/agent-token)" = 0
case "$(stat -c %a /etc/rancher/k3s/agent-token)" in 400|600) ;; *) echo 'agent-token must be mode400 or600' >&2; exit 1;; esac
test -s /etc/rancher/k3s/config.yaml.d/90-join.yaml
CHECK
chmod 755 /usr/local/sbin/k3s-worker-preflight
cat > /etc/systemd/system/k3s-agent.service.d/10-preflight.conf <<'UNIT'
[Unit]
ConditionPathExists=/etc/rancher/k3s/agent-token
ConditionPathExists=/etc/rancher/k3s/config.yaml.d/90-join.yaml
[Service]
ExecStartPre=/usr/local/sbin/k3s-worker-preflight
UNIT
cat > /etc/rancher/k3s/join.yaml.example <<'CONF'
# Copy to config.yaml.d/90-join.yaml only after Pi5 server is ready.
# Replace node-ip with the verified stable Pi4 wired address.
server: https://192.168.1.133:6443
token-file: /etc/rancher/k3s/agent-token
flannel-iface: eth0
# node-ip: REPLACE_WITH_PI4_WIRED_IP
CONF
chmod 600 /etc/rancher/k3s/join.yaml.example
systemctl daemon-reload
# Test explicitly: must refuse Wi-Fi-only startup, without starting k3s.
if /usr/local/sbin/k3s-worker-preflight; then echo 'Unexpected preflight success'; exit 1; else echo 'Preflight correctly refuses unconfigured/unwired join'; fi
unshare --net bash -c 'set -e; d=$(find /var/lib/rancher/k3s/data -mindepth 2 -maxdepth 2 -type d -name bin | head -n1); "$d/aux/iptables-nft" --version; "$d/aux/iptables-nft" -t filter -A FORWARD -j ACCEPT; "$d/aux/iptables-nft" -t nat -A POSTROUTING -j MASQUERADE; echo BUNDLED_NFT_SMOKE_OK'
