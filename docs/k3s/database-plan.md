> Historical proposal. Production is now native k3s: OAuth/Tipp production DBs
> are on Pi5 SSD-backed ext4 state; development DBs are on Pi4 SSD. All retain
> their single-writer StatefulSets and restored content. See README.md for the
> current topology; the inventory below describes the pre-migration source.

# Database plan for the two-Pi migration

Research and proposal, 2026-10-29. No database or storage was changed.

## Observed inventory

Current MariaDB workloads are independently managed StatefulSets:

| Namespace | StatefulSet | PVC | Initial target |
|---|---|---|---|
| tipp | prod-db | prod-db | Pi5, one writer |
| oauth | db | db | Pi5, one writer |
| tipp | dev-db | dev-db | Pi4 optionally; accept downtime |
| media | dev-db | dev-db | Pi4 optionally; accept downtime |

All four currently use the same pinned MariaDB image digest
`sha256:c4124d1afa875e47f2892edf26b482fa54f796e2f342d3717a04ca091a22a9bd`.
No Fdatabase custom resources were returned by the live inventory. The database
controller's default new-workload path still uses a single Deployment/Recreate,
hardcoded standard/2Gi/RWX PVC, and is not a replication operator. It deliberately
refuses to start a second writer beside an independently managed same-name StatefulSet.
Do not recreate production DBs as Fdatabase resources during this migration.

The four PVs currently use Minikube hostPaths under
/tmp/hostpath-provisioner/<namespace>/<name>, class standard, reclaim Delete,
and no node affinity. They are node-container paths, not evidence of portable or
shared storage. Their data will not appear automatically in a new cluster.
PVC removal can delete backing data; preserve all old objects and backup material.
Read-only findmnt confirms the production Tipp (~176MiB) and OAuth (~162MiB)
datadirs currently live on Pi5 SD-backed Docker volumes. These are approximate
on-disk sizes at inspection, not dump sizes or target capacity requirements.
The inspected Minikube etcd path was also SD-backed at that moment; separate
maintenance may change it, so re-inventory before cutover.

## Recommended initial architecture

1. Separate **k3s cluster state** from **application databases**. A single Pi5 k3s
   server can use embedded SQLite, with consistent backups and its server token.
   Do not point k3s at a MariaDB pod inside its own cluster: that introduces a
   bootstrap dependency. Embedded etcd HA requires at least three server nodes;
   a Pi4 worker does not supply database or control-plane quorum.
2. Keep each existing production database as one MariaDB StatefulSet replica,
   explicitly pinned to Pi5, each with its own retained local volume. Existing
   application Service DNS names (e.g. prod-db.tipp.svc.cluster.local and
   db.oauth.svc.cluster.local, after verifying actual service names) remain the
   application interface. App replicas on Pi4 connect privately over cluster networking.
   Keep credentials in Secrets and verify NetworkPolicy enforcement and grants.
3. New persistent storage should be native Linux filesystems with static local
   PVs, ReadWriteOnce, Retain, filesystem identity guards and required Pi5 node
   affinity. Never run two independent MariaDB processes on one datadir or treat
   RWX/NFS as database replication. Storage must not silently fall back onto the SD
   when an SSD/image mount disappears.
4. Owner requires SSD layout to stay as-is and HDD work to wait. Therefore the
   preparation does not allocate, resize, format, or repurpose SSD images. Initial
   new k3s state and database restore targets use Pi5's existing ext4 SD instead;
   observed root free space ~120GiB. Database restore parent directory
   /var/lib/fabkli/database-restore is staged, empty, mode0700. Do not copy existing
   DB files into it live. Allocate each target separately, budget capacity and use
   Retain/local PV node affinity. This preserves the current SD storage class of
   the databases; it does not deliver the later intended SSD performance benefit.
   Prefer a verified native Linux SSD location later, through its own backed-up
   migration. Until isolated restore and capacity/I/O tests pass, this is a staged
   design rather than evidence of production migration readiness.
5. Pi4 optional development DBs can use bounded local SD storage, with affinity
   and retained volumes. They stop with Pi4. Production never depends on them.
   Keep monitoring retention, download staging and image storage within one shared
   SD budget; the same free bytes cannot be promised independently to every workload.

Expected behavior: Pi4 loss stops Pi4-only work, while surviving Pi5 app replicas
and databases keep serving. Pi5 loss interrupts production until recovery. This
is useful application redundancy but **not database or whole-system HA**.

## Backup and migration procedure

- Inventory DB versions, datadir mounts, engines, size, routines/events/triggers,
  users/grants, application migration versions and all writers/cron consumers.
  Preserve the current image digest; separate database upgrades from migration.
- Existing daily backup evidence was verified by other maintenance today, but an
  independent/off-device destination remains unspecified. Attached SSD copies
  protect against some mistakes, not losing the host/drives. Select encrypted
  independent storage and retention; do not publish credentials in source/TODO.
- Restore database-consistent dumps into isolated scratch databases and validate
  schema, row counts/checksums appropriate to the apps, grants, login and a test
  transaction. Restore routines/events/triggers where applicable; prevent duplicate
  scheduled events/jobs in the scratch environment. A successful archive hash is
  not proof of a successful restore.
- Before cutover, pause each application's writes, workers, schedulers and imports;
  take the final consistent backup. Stop the old DB writer before starting its
  replacement against migrated state. A live filesystem copy is not a consistent
  MariaDB backup. Never let old and new environments diverge unnoticed.
- Restore to distinct new retained volumes; verify UID/GID, image version,
  datadir identity, probes and SQL health. Start one application replica, verify
  critical flows, then enable the second app replica after its shared-state audit.
- Re-enable jobs once; test restart and backup restoration. Agree actual RPO/RTO
  after measuring dump/restore duration and acceptable downtime.
- Rollback: stop new writers before old recovery. If new writes occurred, explicitly
  reconcile/export them or accept a documented loss window; switching back to an
  old snapshot is not automatically lossless. Keep old Minikube volumes untouched
  until acceptance and a separately deliberate retirement.

## Replication options later

| Option | Benefit | Limitation / decision |
|---|---|---|
| Single primary Pi5 + tested independent backups | Lowest operational cost; Pi4 can disappear | No automatic Pi5 recovery; recommended initial state |
| Primary Pi5 + async replica Pi4 | Additional recoverable copy / optional reads | Lag/data loss possible, SD capacity and I/O cost; promotion requires fencing and deliberate failover |
| Semi-sync replica | Can improve durability depending on wait/fallback configuration | Adds latency and dependency; timeouts may fall back to async; still needs fencing/failover management |
| Galera with two data nodes + independent third arbitrator | Quorum with two data copies | Needs reliable third failure domain; arbiter has no data; complexity and Pi4 SD performance make this a later project |
| Three suitable data nodes / external managed DB | Path toward database HA | Additional hardware/service, storage, tested recovery and routing required |

Do not configure automatic two-node promotions without fencing: network isolation
must never produce two writable primaries. An arbitrator on either existing Pi
is not an independent third failure domain. A phone is not assumed reliable or
available. Even true DB replication does not replace versioned backups.

Shared sessions/signing keys, uploads, rate limits, consumers and schema migration
execution must be audited separately in Tipp/OAuth. Operator placement cannot solve
application-level shared-state bugs or duplicate background jobs.

## Primary references

- K3s datastores and constraints: https://docs.k3s.io/datastore
- K3s SQLite backup includes DB directory and server token: https://docs.k3s.io/datastore/backup-restore
- K3s server quorum: https://docs.k3s.io/datastore/ha-embedded
- MariaDB replication tradeoffs: https://mariadb.com/docs/server/ha-and-performance/standard-replication/choosing-a-replication-strategy
- Galera quorum: https://mariadb.com/docs/galera-cluster/high-availability/understanding-quorum-monitoring-and-recovery
- Two data nodes plus independent arbitrator: https://mariadb.com/docs/galera-cluster/galera-architecture/galera-cluster-deployment-variants/
