# Migration day runbook

Preparation and release publication do not authorize rollout. No command in local test targets reads the current kubeconfig. The release pipeline publishes artifacts only; there is no deployment job. Downtime is allowed on migration day.

## Ownership and versions

The operator owns same-namespace HTTPRoutes, Services, application Deployments and dedicated ServiceAccounts. Cluster infrastructure owns Gateway API CRDs, Envoy Gateway, GatewayClass, Gateway/listeners, traffic policies, NodePort exposure and the Grafana route. Helm v1.9.2 owns its included Gateway API v1.6.1 and Envoy extension CRDs; do not also install a separate standard-install.yaml.

Pinned data plane: Envoy distroless-v1.39.2, ARM64/AMD64 index sha256:dced08cf7c472e1a1d067f906878266078eeeb63c110b4961882c039a622853a. Controller-runtime v0.24 uses the Kubernetes v0.36 client series; the live API server is v1.35. The isolated API-server test uses v1.35.0. Gateway API Go structs are v1.6.1, matching the chart CRDs.

References: [Envoy compatibility](https://gateway.envoyproxy.io/news/releases/matrix/), [v1.9.2 proxy patch guidance](https://gateway.envoyproxy.io/news/releases/notes/v1.9.2/), [proxy configuration](https://gateway.envoyproxy.io/docs/tasks/operations/customize-envoyproxy/).

## Before cutover

1. Record the old operator image digest, all owned Ingress YAML, Gateway API resource absence/presence, host NGINX configuration and app/PVC inventory. Back up app databases consistently. Do not delete CRDs, PVCs, PVs or recreate Minikube.
2. Download the release install.yaml, image.txt, gateway-config.tar.gz and SHA256SUMS. Verify checksums and the multi-architecture image. Save the old operator installation artifact for rollback.
3. Review live Fdeployment tags/resources/probes and security requirements. Prefer versioned tags; existing development latest tags remain accepted. Before upgrading the operator, follow [NGINX compatibility rollout](nginx-compatibility.md): update the CRD and frontend security fields first. Do not deploy v0.2.1 against standard root-master NGINX frontends. Root images remain supported for compatibility. Privileged mode is false by default, capabilities are dropped and seccomp uses RuntimeDefault. Set spec.security.runAsNonRoot=true only for tested nonroot images; spec.security.privileged=true is an explicit exceptional compatibility setting. Test images before rollout. Do not create blanket deny NetworkPolicies without checking the CNI and DNS/database/Envoy flows.
4. Ensure capacity for the controller and proxy requests (combined 200m CPU / 384Mi). Install the pinned chart on the explicitly selected migration context:

   ```sh
   helm upgrade --install eg oci://docker.io/envoyproxy/gateway-helm --version v1.9.2 --namespace envoy-gateway-system --create-namespace --values config/gateway/values.yaml --kube-context minikube
   kubectl --context minikube apply -f config/gateway/gateway.yaml
   kubectl --context minikube apply -f config/gateway/grafana-route.yaml
   ```

If monitoring migration is deferred, omit grafana-route.yaml and retain Grafana’s
legacy Ingress and the ingress controller until that separate migration.

5. Inspect GatewayClass Accepted, Gateway Accepted/Programmed and listener conditions. Discover the generated data-plane Service and its HTTP NodePort using owning-gateway labels. Do not confuse the controller Service with this Service. Review allowed route namespaces before adding new apps. Keep NodePort off the public internet.
6. Apply the digest-pinned release install.yaml only on migration day, with cleanup-legacy-ingress=false (default). Wait for operator readiness, then every HTTPRoute parent Accepted/ResolvedRefs at current generation, workload replicas and endpoints.

## Traffic verification and cutover

Host NGINX/Certbot retain TLS, renewal and HTTP-to-HTTPS redirects. Change only application upstreams from http://192.168.49.2 to the selected http://192.168.49.2:NODEPORT after testing. Keep API-server, Jellyfin, phpMyAdmin and unrelated host routes unchanged. Back up files and run nginx -t before reload. Keep forwarded Host, X-Real-IP, X-Forwarded-For, X-Forwarded-Proto, X-Forwarded-Host and X-Forwarded-Port; Envoy trusts one forwarding hop. The trusted-hop model requires NodePort exposure restricted to the host/internal network.

Test each original Host header through NodePort first, including /api and / on shared hosts, /api/child, /apix, query strings, backend-specific health content, forwarded scheme, WebSockets and uploads/timeouts. A frontend 200 at a backend health URL is not proof of backend health. Compare the original paths and response bodies. Verify public HTTPS with valid TLS and redirects, then test from an independent LAN client. The supplied BackendTrafficPolicy disables the total upstream response timeout and
sets a 60-second stream idle timeout to preserve the previous ingress idle-timeout
behavior. Host NGINX retains its 60-second proxy read timeout. Validate long-running
requests and uploads before cutover; a 17-second response and WebSocket message echo
were verified in the recorded live migration.

Keep tipp.internal.fabkli.ch internal: do not add it to public host NGINX or public DNS/router exposure. Update the Pi cronjob's internal endpoint to the data-plane NodePort while preserving its Host header. Inspect cronjob configuration separately; do not silently make it public.

Once traffic has switched and passed verification, explicitly set --cleanup-legacy-ingress=true on the operator. It deletes only same-name, correctly owned legacy Ingresses after the selected Gateway/listener and HTTPRoute are current and ready. It cannot prove external traffic cutover; enabling the flag is the administrator's acknowledgment. Grafana's independent legacy Ingress must be retired separately. Disable the cluster ingress addon only after all its remaining consumers have migrated; leave host NGINX running.

## Data safety and Popeye findings

There are currently no Fdatabase CRs; live MariaDB StatefulSets are independent. They are never adopted or converted. The controller refuses to create a same-name database Deployment beside a StatefulSet. Operator database PVCs are retained when the CR is deleted; existing owned PVCs are detached before finalization. Keep retained PVCs and validate backup/restore before reusing them. Creation still uses the existing standard/2Gi/RWX storage convention; changing storage class, access modes or layout needs its own data migration. Existing managed database Deployments keep image, volumes and security while environment changes use Recreate to avoid overlapping writers.

Popeye is an optional read-only audit tool, not a workload to embed in the operator. Run it manually before and after migration. Findings controlled by this operator are addressed through resource validation, restricted privilege, token automount, real readiness and dedicated application ServiceAccounts. System RBAC, monitoring resource budgets/retention, independent databases, network-policy enforcement, HA and unrelated latest images need separate infrastructure/application changes. Do not auto-fix them globally.

## Rollback

Leave old Ingresses and the cluster ingress controller serving until new traffic succeeds. If cutover fails, restore the saved application upstreams and validate/reload host NGINX, then restore the old operator digest. If owned legacy Ingresses were cleaned, restore the saved Ingress manifests before switching traffic back. Do not uninstall f-operator CRDs: that would delete custom resources. Keep the new security fields on CRs only while supported by the installed CRD; the old operator ignores unknown spec fields. Database PVC retention is intentional and must not be reversed by adding garbage-collection ownership. Envoy infrastructure can remain unused while investigating; do not remove shared CRDs used by other resources.
