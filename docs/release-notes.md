Envoy Gateway routing and operator safety improvements, prepared for a later migration.

- Replace operator-created Ingresses with Gateway API HTTPRoutes, preserving hostnames, prefix paths and Service port 80.
- Configure Gateway attachment explicitly; report current workload and route readiness. Keep legacy Ingresses until the traffic cutover gate is explicitly enabled.
- Refuse unrelated resource adoption, reconcile without unnecessary writes, repair malformed containers, validate resources without panicking, remove workload dumps and fake production recorders.
- Disable blanket privileged containers and token automount; provide explicit security opt-ins and less aggressive probe timeouts.
- Retain database PVCs, remove old CR ownership safely, prevent a second writer beside existing StatefulSets, use Recreate for managed database Deployments and fix swapped resource units.
- Update compatible Kubernetes/controller-runtime/Gateway API dependencies, Go patch toolchain and generation tools.
- Preserve Kubernetes API defaults to avoid repeated Deployment/HTTPRoute patches.
- Cap local verification memory and CPU, disable test-container swap, and run image builds on GitHub.
- Test with an isolated loopback API server and dummy resources; publish ARM64/AMD64 images and digest-pinned installation artifacts.

This release does not install Envoy or deploy the operator. The pipeline has no deployment job or cluster credentials. Read migration.md before migration day. New security defaults can require an explicit application compatibility exception. Existing independent StatefulSets are not taken over.

Release publishing uses the existing CR_PAT repository secret for the existing GHCR package, with GITHUB_TOKEN as a fallback. The v0.2.0 tag built successfully but publication was rejected by package permissions; v0.2.1 includes this authentication correction. No v0.2.0 GitHub release was published.
