# Live Envoy migration — 2026-10-07

Application traffic was migrated to Envoy Gateway v1.9.2 with the pinned Envoy
v1.39.2 image and f-operator v0.2.2. All eleven Fdeployments have current Available
conditions and HTTPRoutes with current Accepted/ResolvedRefs conditions. All
application replicas rolled out using the same image digests as before migration.

Host NGINX still terminates TLS and redirects HTTP to HTTPS. Only Keys, Minecraft,
Media development, OAuth, Tipp development and Tipp production upstreams changed
to the generated HTTP NodePort (30492 on this cluster). The internal Tipp script
uses that NodePort with its original Host header; no public NGINX server was added
for the internal hostname. Other host upstreams and Certbot settings are unchanged.

Monitoring was explicitly excluded. Grafana's original Ingress remains, as does
ingress-nginx; do not disable the addon while Grafana depends on it. No monitoring
workload/chart/route was migrated. Once cutover tests passed, operator legacy cleanup
was enabled on the live Deployment and removed its eleven owned application
Ingresses. The release's default cleanup setting remains false for safe new installs.

Five root-master NGINX resources need nginxCompatibility: Keys plus the four live
frontends. The live CRs were patched before the new operator started. Image tags,
replica counts, application environment and independent database StatefulSets were
preserved. Original configuration and consistent database dumps were saved privately
on the Pi; private snapshots and SQL dumps are not committed to this repository.

## Verification

- Every non-completed pod Ready; completed CronJob/admission pods are expected.
- All real application readiness URLs returned HTTP 200, including both production
  Tipp replicas, the internal consumer and the independently managed image service.
- Eighteen public HTTPS status/content checks matched the prior ingress; frontend
  and API response bodies were identical. Minecraft intentionally returns random
  JSON, so compare its status/content type/schema rather than byte equality.
- Six public hostnames worked using normal DNS and valid TLS. HTTP redirects survived.
- Referenced frontend JavaScript/CSS assets matched between both proxies before
  old application Ingress cleanup. API and /apix prefix precedence remained correct.
- A disposable 64MiB, half-CPU, non-root, read-only/no-capabilities echo container
  tested forwarded HTTPS/Host headers, encoded queries, a 2,228,224-byte upload,
  WebSocket upgrade and message echo, and a 17-second response. Its temporary route,
  endpoint/service and container were removed after testing.
- BackendTrafficPolicy removes Envoy's shorter total response timeout and retains a
  60-second stream idle timeout. This policy is necessary migration infrastructure;
  use the updated gateway.yaml as well as the v0.2.2 operator installation artifact.

These are protocol, asset and health checks from the Pi, not full browser login or
an independent LAN-device test. No synthetic request triggered internal game imports,
notifications or application writes. Normal database/application operation continues.

## Persistence and rollback

Helm owns the Envoy/Gateway API CRDs. Keep the release values and gateway.yaml in
this repository; never delete f-operator CRDs or application PVCs to roll back.
The Pi's migration directory stores original operator/Ingress/host NGINX manifests,
CRs, database dumps and test evidence. Since old application Ingresses are now gone,
restore their saved manifests before restoring original host upstreams, then restore
the old operator installation if needed. Keep Grafana on its existing route until
its separately authorized monitoring rework.
