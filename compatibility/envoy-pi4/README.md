# Envoy on Pi4 with a 39-bit kernel

The official Envoy v1.39.6 ARM64 binary aborts on this Pi4 kernel because its TCMalloc build assumes a 48-bit virtual address space. The compatibility build keeps the exact upstream release commit and distroless runtime image, replacing only the Envoy binary built with `--define=tcmalloc=gperftools`.

Dispatch the existing `security-publish.yaml` workflow **on this branch only**. It builds on an isolated GitHub ARM64 runner and does not deploy, change production, enable frozen deployment workflows, or modify the Pi4 kernel. The hosted build can take several hours and has a six-hour bound. The default branch security-publication workflow is untouched.

Before deployment: verify source/build provenance and binary checksum, run the image as an isolated no-network non-root Pi4 container with `--version`, and validate a representative Envoy configuration on Pi4. Then configure the owning EnvoyProxy to use the immutable image digest, restore its original two-node placement, wait for both replicas, and verify actual ingress traffic on both nodes and Pi4-loss behavior. Keep Pi5 ingress serving during the rolling replacement; preserve the prior parent configuration for recovery. Do not claim Pi4 compatibility from a hosted-runner smoke alone.

Upstream explanation: https://github.com/envoyproxy/envoy/issues/23339
