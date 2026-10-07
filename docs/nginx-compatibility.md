# Standard NGINX frontend compatibility and rollout

The v0.2.1 operator drops all Linux capabilities. The live root-master NGINX images fail during cache-directory chown under that configuration. Use v0.2.2 or later with this explicit Fdeployment option:

```yaml
spec:
  security:
    nginxCompatibility: true
    privileged: false
    runAsNonRoot: false
```

This grants only CHOWN (cache/file ownership), SETUID and SETGID (switch workers to the nginx user), and NET_BIND_SERVICE (listen on port 80). All other capabilities remain dropped. Privilege escalation remains disabled, seccomp remains RuntimeDefault, and service-account token mounting remains disabled. The master stays root; this is not a nonroot-image conversion. Enabling nginxCompatibility together with runAsNonRoot is rejected by CRD validation and controller validation. Non-NGINX backends keep the existing restrictive default.

## Prepared source manifests

| Repository | File | Target |
| --- | --- | --- |
| tipp | frontend/resources/fdeployment-dev.yaml | tipp/dev-frontend |
| tipp | frontend/resources/fdeployment-prod.yaml | tipp/prod-frontend |
| f-medias | frontend/resources/fdeployment-dev.yaml | media/dev-frontend |
| f-medias | frontend/resources/fdeployment-prod.yaml | media/prod-frontend (not currently deployed) |
| f-oauth | frontend/resources/deployment.yaml | oauth/oauth-frontend |

Prepared draft PRs: [Tipp #208](https://github.com/FabioKaelin/tipp/pull/208), [Media #3](https://github.com/FabioKaelin/f-medias/pull/3), [OAuth #39](https://github.com/FabioKaelin/f-oauth/pull/39).

Only the security fields change in these files. Do not blindly apply their historical tags, replica counts or environment values to live resources. Production pipelines normally substitute versions; the source files and live resources differ. Existing development latest tags remain accepted for compatibility; pin versions separately when ready. Frontend changes are on preparation branches/PRs because merging into main/develop would trigger automatic cluster deployment.

## Migration-day order (not executed during preparation)

1. Download/verify the operator release artifacts, save old CRD/operator manifests and current Fdeployment specs. Prepare Envoy infrastructure as described in migration.md. Do not deploy v0.2.1 against these frontend images.
2. Save the old operator replica count and pause its Deployment before changing the CRD/CRs. This prevents an older controller from dropping unknown spec fields during an update. Existing application pods and ingress keep running; reconciliation is temporarily paused. On migration day only: `kubectl --context minikube -n f-operator-system scale deployment f-operator-controller-manager --replicas=0`. Wait for its old controller pod to terminate. Then apply only the updated Fdeployment CRD first, from the checked-out v0.2.2 tag: `kubectl --context minikube apply -f config/crd/bases/k8s.fabkli.ch_fdeployments.yaml`. Wait for the CRD Established condition. This makes the new security field persist before starting the new controller.
3. For the four currently deployed frontend resources, patch only security fields, preserving their image tags, replicas, resource budgets and environment. Commands below are for migration day only:

   ```sh
   kubectl --context minikube -n tipp patch fdeployment dev-frontend --type=merge -p '{"spec":{"security":{"nginxCompatibility":true,"privileged":false,"runAsNonRoot":false}}}'
   kubectl --context minikube -n tipp patch fdeployment prod-frontend --type=merge -p '{"spec":{"security":{"nginxCompatibility":true,"privileged":false,"runAsNonRoot":false}}}'
   kubectl --context minikube -n media patch fdeployment dev-frontend --type=merge -p '{"spec":{"security":{"nginxCompatibility":true,"privileged":false,"runAsNonRoot":false}}}'
   kubectl --context minikube -n oauth patch fdeployment oauth-frontend --type=merge -p '{"spec":{"security":{"nginxCompatibility":true,"privileged":false,"runAsNonRoot":false}}}'
   ```

   Read the CRs back and confirm nginxCompatibility persists. Do not create media/prod-frontend merely because a source manifest exists.
4. Deploy the digest-pinned v0.2.2-or-later operator install.yaml with legacy Ingress cleanup disabled. The release restores the operator replica and starts the new controller. This controller applies the capability profile to frontend Deployments. Confirm new pod security settings, no startup errors, ready probes, frontend HTML and static assets, and API requests through original and Envoy routes. Complete public HTTPS/LAN checks before traffic cutover or legacy cleanup.
5. Merge the prepared frontend manifest PRs at the coordinated rollout time, accounting for their automatic image-build/deploy workflows and selected versions. Future frontend deploys then retain the option. Do not merge solely to update documentation while deployment is deferred.

## Local evidence and verification limits

On 2026-10-07 all four live ARM64 image digests were tested sequentially in standalone Docker containers with 64MiB RAM, no swap, 0.5 CPU, no-new-privileges and all capabilities dropped. Startup failed at chown without the option; with the four allowed capabilities each served frontend HTML successfully without permission/emergency logs:

- medias-frontend: sha256:9c56f4a79752d43e0b4d9b9774837f09f2251ddda6e2ec820e5f6ae8324cf761
- oauth-frontend: sha256:50d9c992a4e6dfe91ecc1e19ab186ce164dba755dced85a5cb1d84802b17963a
- tipp-frontend development: sha256:87d7cac75e6e6898762dae18775a4ba0ebabc393a0bdbfc4c0312a2d53f03bd8
- tipp-frontend production 1.3.1: sha256:6433cf77a033a17935417f8de6d574516bb2272086bca4a02e501d3fd9c409eb

Use `python3 hack/test-nginx.py --image IMAGE@DIGEST` to repeat bounded startup/HTTP checks. CI also tests nginx:stable and nginx:stable-alpine. Controller tests verify opt-in, opt-out, retained restrictions and rejection of conflicting nonroot settings. These tests do not establish live cluster rollout, complete frontend/browser behavior or future image compatibility. Retest changed images. No cluster manifests were applied during preparation.

Rollback: retain the old controller image and live Fdeployment specs; restoring the previous operator reverts to its prior behavior. Keep legacy ingress serving until cutover is verified. Preserve data/PVCs. Avoid rolling back to v0.2.1 with these standard NGINX images.
