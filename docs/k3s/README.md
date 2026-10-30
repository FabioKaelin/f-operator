# Two-Pi native k3s deployment

Production moved to native k3s on October30–31, 2026. The historical preparation
notes below describe the earlier state and must not be used as current topology.
The prepare scripts are one-time, pre-cutover tools; do not rerun them on joined nodes.

- `raspy5`, wired192.168.1.133: one control-plane server; production OAuth/Tipp
  databases and durable app state in a96GiB ext4 image on the existing4TB exFAT SSD.
- `raspy4`, wired192.168.1.117: worker with1TB native ext4 SSD for downloads,
  monitoring, development databases and worker app state.
- Pi5 native ext4 16TB HDD is the first writable media branch; the old SSD media
  branch does not accept new creations. A verified background mover is consolidating
  movies, series and other media onto HDD. Keep application paths unchanged.
- Both nodes have completed successful postboot storage/cluster checks. Original
  Minikube is stopped and retained, including its data and context. Never prune it.
- Single-writer MariaDB StatefulSets were restored from validated cold data and
  SQL exports; no replication or control-plane HA is implied by two worker nodes.
- Backups retain SQL, native SQLite-consistent snapshots, app state, Kubernetes
  exports and host configuration, encrypted and verified on Pi5, HDD and Pi4 SD.
- Placement is declared by `k3s-workload-placement` in `f-operator-system`.
  Native delivery must preserve storage mounts, guards, resource limits and replica
  placement. Legacy Minikube etcd and pressure-policy workflows refuse to alter it.
- Pi4-compatible Envoy allocator build and final delivery acceptance are tracked
  in `/home/admin/todo.md`; ingress remains on Pi5 until compatibility validation.
- Optimizer stays paused. SSD reformat/simplification and dashboard redesign are
  explicitly deferred; media relocation does not authorize formatting app storage.

## Historical preparation checkpoint (October29)

# Two-Pi k3s preparation — 2026-10-29

Status: Pi4 worker prerequisites prepared; production remains Minikube on Pi5.
No production cutover, database migration, SSD reformat, media relocation or operator deployment is part of this preparation.
The HDD migration is deferred. Phone investigation is recorded in /home/admin/todo.md.

## Node preparation and boundaries

Pi5 server binary/config are staged and disabled behind a cutover marker gate;
production kubectl/crictl/ctr tools were preserved (installer symlinks skipped).
No cluster state was initialized and no workloads started. Its initial data-dir
/var/lib/rancher/k3s and /var/lib/fabkli/database-restore are on existing ext4 SD,
which respects the SSD-as-is constraint. This is a capacity-conscious interim
choice, not an SSD durability/performance claim. Pi5 swap remains unchanged
for running production; disable/reconfigure it deliberately at cutover.

Pi5 wired address observed: 192.168.1.133; Wi-Fi 192.168.1.141.
Pi4 observed: raspy4.local / 192.168.1.112 on Wi-Fi; Ethernet link is still down.
Reserve wired DHCP addresses before joining. Never put a public IP on cluster ports.

Pi4 has 8GB RAM, a 256GB ext4 OS SD and roughly 221GiB free initially.
Prepared memory cgroups, overlay/br_netfilter, forwarding/bridge sysctls, persistent
swap disable, NFS client utilities and official checksum-verified k3s v1.35.9+k3s1.
The worker service is installed but deliberately disabled: there is no k3s server
endpoint/token yet. It is not a joined/Ready node. No standalone Pi4 server was created.

Installed configuration: /etc/rancher/k3s/config.yaml. Reservations: system 250m CPU,
512Mi RAM,20Gi ephemeral; Kubernetes 250m CPU,256Mi RAM,2Gi ephemeral.
Evictions retain 15% free disk and 256Mi available RAM; logs bounded to 3x10Mi/container.
Directories /srv/sabnzbd/{incomplete,complete} are prepared, not mounted/shared or in use.
No monitoring/SAB downloads have started. Budget eventual staging at 80–100GiB;
retain >=30GiB OS reserve and use application low-space pause/queue limits too.
Kubelet eviction is not a quota for hostPath downloads. Do not enable downloads
until a quota/budget enforcement design and UID/category mappings are verified.

Host before-copies: Pi4 /var/backups/k3s-preparation-20261029.
Local script/evidence: /home/admin/.local/state/k3s-preparation-20261029.
NFS client installation enabled rpcbind; it was disabled again because no NFSv3
service is required for preparation. No exports or open cluster firewall rules created.

## Placement API

No placement field preserves existing externally configured scheduling. Opting in
makes the operator manage nodeSelector, affinity and rollout strategy. Removing
placement after opt-in clears these owned settings and returns to default scheduling.

Pin one or several replicas to a specific node (replicas remain your choice):

```yaml
spec:
  replicas: 1
  placement:
    mode: SingleNode
    node: raspy5  # actual kubernetes.io/hostname label; verify at migration
```

Spread with fallback to the remaining node:

```yaml
spec:
  replicas: 2
  placement:
    mode: Distributed
    spread: Preferred
    nodeSelector:
      fabkli.ch/production: "true"
```

Preferred is best effort, not a promise of separate nodes. Use `spread: Required`
for at most one replica per hostname. With two replicas and one eligible node,
one stays Pending. Required uses maxSurge=0/maxUnavailable=1 so upgrades do not
wait forever for an impossible third node; with one replica this can cause downtime.
The anti-affinity selector uses stable application labels across image versions,
and is namespace-scoped. nodeSelector restricts eligible nodes; it cannot create capacity.
Existing immutable selectors, injected sidecars and unrelated metadata are retained.
SingleNode uses the scheduler and hostname label, not spec.nodeName bypassing scheduling.

All replicas need matching signing/session configuration, shared required state,
correct readiness probes and safe job/migration execution. Audit Tipp/OAuth before
scaling; two replicas alone do not implement application HA. Pi5 must serve alone.
Application PDBs should be chosen during rollout based on actual replica counts;
none are silently added to existing applications by this change.

## Operator redundancy

Default manager remains one replica with leader election enabled.
`config/overlays/ha` renders two replicas, preferred hostname anti-affinity and a
minAvailable=1 PDB. Both managers share the existing leader election lease; only
one reconciles at a time. Verify regcred, image publication and Gateway API CRDs
on the destination before deployment. Render with:

```sh
bin/kustomize build config/overlays/ha
```

This is a prepared overlay, not applied to production. A PDB covers voluntary
evictions, not node/power failure; preferred anti-affinity permits one-node operation.

## Pi5 server and migration sequence

1. Recheck current inventory, backup consistency/restore evidence and host/API health.
   Initial API requests intermittently timed out; later readyz and reads succeeded.
   Fresh node reports v1.35.10 (other maintenance changed it during this session).
   Keep source changes isolated from that maintenance and use an explicitly pinned
   matching k3s minor; do not downgrade the existing production Kubernetes.
2. Establish wired links/reservations. Preserve host NGINX, internal Tipp exposure,
   Envoy/Gateway, pressure guard, secrets, priority classes and all storage guards.
3. Initial state storage is the existing Pi5 native ext4 SD; root had ~120GiB
   available at inspection. This avoids modifying the SSD. Verify capacity, I/O
   under load and independent backups before activation. Existing SSD is exFAT with guarded Linux images,
   observed ~119GiB outer free during this preparation. Do not put SQLite/etcd
   directly on exFAT. No new image, resize or format was performed. A later SSD move needs a separately chosen verified
   native ext4 mounted path and identity guard, not an occupied-drive reformat.
   Existing encrypted etcd storage belongs to Minikube and must not be reused live.
4. Activate the staged Pi5 server with Traefik and ServiceLB disabled to preserve chosen Envoy
   and host ingress design. Keep NetworkPolicy enforcement enabled and test it.
   Bind API/private VXLAN to LAN; allow6443 Pi4→Pi5,8472 UDP node↔node and10250
   node↔node only. Pick non-overlapping pod/service CIDRs. Never install a second
   etcd server as an attempted two-machine HA design.
5. K3s+Minikube overlap duplicates RAM/CPU demand and NodePort/hostPort ownership.
   Test only a bounded temporary workload, with distinct ports; do not enable all
   workloads twice. Prepare a maintenance cutover if overlap capacity is inadequate.
6. Populate Pi4 server endpoint and root-owned token file, verify CA trust, set
   flannel-iface/node-ip to the verified wired interface/address, then enable agent.
   Label both intended application nodes fabkli.ch/production=true. Test DNS,
   cross-node connectivity and NetworkPolicy before application rollout.
7. Restore databases into distinct retained local volumes, one writer at a time.
   Copy/export CRDs, CRs, Helm state and declarative resources; rebuild/publish local
   images. Preserve storage identities and exact media paths. See database-plan.md.
8. Test routing/session behavior, download/import/playback, backups and an explicitly
   coordinated Pi4 outage. Verify Pi5 DNS/gateway/databases/essential apps remain usable.
9. Keep Minikube and backups until verified cutover; rollback must stop new writers
   before returning to old state, with an explicit plan for writes since cutover.

References: https://docs.k3s.io/installation/requirements ;
https://docs.k3s.io/installation/configuration ; https://docs.k3s.io/datastore ;
https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#inter-pod-affinity-and-anti-affinity

## Worker startup guard and checker evidence

Pi4 /etc/systemd/system/k3s-agent.service.d/10-preflight.conf requires a root-owned
mode400/600 token and config.yaml.d/90-join.yaml. Its ExecStartPre checks memory
cgroups, swap disabled and a real IPv4 wired eth0 link. Current Wi-Fi-only
preflight correctly refuses startup. join.yaml.example is staged without a token.

After reboot the k3s checker reports three missing **legacy** IP_NF_FILTER,
IP_NF_NAT and IP_NF_TARGET_MASQUERADE symbols on kernel6.18. This is a failed
legacy-symbol check, not a clean checker pass. Actual host iptables-nft
filter/conntrack/NAT and IPv6 filter operations passed in isolated namespaces;
the k3s bundled aux/iptables-nft filter/NAT smoke also passed. Default
prefer-bundled-bin=false is retained. These checks do not replace joined-node
Flannel, service routing or NetworkPolicy verification.

The three shell scripts here record the exact preparation performed. They are
bootstrap records, not a promise of safe repeated execution on an existing cluster.
Pi5 script deliberately refuses an existing k3s configuration/service. Review
addresses, version, paths and available capacity before reuse.

## Preparation rollback and verification limits

Preparation did not initialize a Pi5 server or join a Pi4 agent. Keep their services
disabled while reviewing; do not run the installed killall/uninstall helpers on
Pi5 as a shortcut during a live Minikube session. To undo OS preparation on Pi4,
restore its backed-up cmdline, remove only the task's swap/modules/sysctl drop-ins,
and reboot; confirm SSH first and retain the backups. Prepared empty staging
folders contain no migrated data. Do not apply this rollback to active workloads.

Inventory also captured enabled host services/timers, CronJobs, CRDs, routes and
Gateways privately. Preserve/replace arr-books, arr-usenet, arr-mounts, Minikube
startup/storage guards, media optimizer policies and backup/security/logging timers
explicitly at migration. A k3s worker installation does not port these integrations.

Tests use fake-client reconciliation plus a loopback envtest API/etcd, not the
production kubeconfig. They verify CRD admission and generated Deployment changes;
they cannot establish physical-node spread, routing failover, or database safety.
Those checks need the wired joined cluster and a coordinated workload rollout.

## Ready-for-cutover checklist

- Pi4 wired link and stable IPv4 confirmed; replace any candidate IPs in configs.
- Readiness/pressure and free-space measurements taken while downloads/playback run.
- Independent encrypted backup destination selected and isolated restores proven.
- Database/application single-writer cutover and rollback window coordinated.
- Minikube-specific integrations replaced, local-only images published or transferred.
- Pi5 swap and port/resource overlap resolved before removing its startup gate.
- Fresh k3s patch/release checked; both new nodes use one tested pinned version.
- Pi5 starts first; transfer its CA-bound token privately; Pi4 joins via wired eth0.
- Verify DNS, NetworkPolicy isolation, cross-node routing, workload spread and Pi4-loss behavior.

No item here is satisfied merely by installing the two k3s binaries.

Both staged kube-proxy configurations restrict future NodePorts to LAN IPv4
192.168.1.0/24. Revalidate this CIDR if the network changes. This is not a
substitute for the node-to-node/API/kubelet firewall rules before activation.
