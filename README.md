# f-operator

Kubernetes operator for Fdeployment application workloads and Fdatabase resources. Application routing uses Gateway API HTTPRoutes attached to an infrastructure-owned Envoy Gateway. See [migration day runbook](docs/migration.md) and [release notes](docs/release-notes.md).

## Local development

Use Go 1.26.8 or automatic toolchain selection. Run `make test` for controller unit tests and a real isolated Kubernetes API-server test with dummy resources. It starts local etcd/API-server processes on loopback; it does not read your kubeconfig or run any Kubernetes workloads on Minikube. `make build` compiles the manager. `make test-envoy` runs an isolated Docker network with standalone Envoy Gateway and dummy HTTP backends, then cleans up those temporary containers. Standalone mode is used only for debugging; production manifests use the Kubernetes provider.

Do not run `make run` against your normal kubeconfig for debugging: it connects a real controller to the selected cluster. Use the isolated test harness instead. `make install` and `make deploy` mutate the selected cluster and are for a separately authorized migration day.

On the Pi, run checks sequentially with a hard memory cap rather than starting unconstrained builds:

```sh
systemd-run --user --scope -p MemoryMax=1200M -p MemorySwapMax=0 -p CPUQuota=100% env GOMAXPROCS=1 GOMEMLIMIT=700MiB GOFLAGS=-p=1 make test
systemd-run --user --scope -p MemoryMax=1200M -p MemorySwapMax=0 -p CPUQuota=100% env GOMAXPROCS=1 GOMEMLIMIT=700MiB GOFLAGS=-p=1 make build
```

The local Envoy test containers have individual hard memory and CPU limits with swap disabled (768MiB combined maximum). Release image builds run on GitHub runners.

## Configuration

Manager flags:

- `--gateway-name=f-operator`
- `--gateway-namespace=envoy-gateway-system`
- `--gateway-listener=http`
- `--cleanup-legacy-ingress=false`: explicit post-cutover cleanup gate; only owned Ingresses with current route/Gateway readiness are deleted.

Fdeployment retains its host, path, image/tag, port, resource and health-check fields. Host and PathPrefix route to the same-named Service on port 80. Use versioned image tags; resource requests/limits must be valid positive quantities with requests no greater than limits. `security.privileged` defaults false; `security.runAsNonRoot` can be enabled for compatible images. All applications disable token automount and default to dropped capabilities/RuntimeDefault seccomp. Dedicated ServiceAccounts are created automatically. Available means current Deployment replicas and the configured Gateway/listener/HTTPRoute are ready.

Fdatabase-created PVCs survive CR deletion. Existing independent StatefulSets are never taken over or duplicated. Review the runbook before using retained database volumes.

## Release

Pushes and pull requests run validation. Semver tags `vX.Y.Z` additionally build ARM64/AMD64 images, publish to GHCR, and create a GitHub release with a digest-pinned install.yaml, image.txt, gateway configuration archive, migration runbook and SHA256SUMS. There is no deployment job and no cluster credentials in this workflow. The default version is 0.2.1. Never reuse published tags.

Pinned infrastructure manifests live in `config/gateway`. The public TLS proxy and certificate renewal remain host-managed. Internal Tipp traffic stays internal. Popeye is optional read-only audit tooling; it is not installed by the operator.
