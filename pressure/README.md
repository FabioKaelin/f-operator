# Owner workload pressure policy

Order in policy.json is first stopped to last. Non-preempting PriorityClasses protect scheduling/eviction order without abruptly preempting database pods. The guard checks physical host MemAvailable and Minikube parent-cgroup headroom every 20 seconds. Three low-memory samples or fifteen >95% busy-CPU samples shed one group. Recovery requires ten healthy minutes and resumes groups in reverse order. It preserves original replica/suspend state with resourceVersion checks and a durable ConfigMap journal before mutations. Applications terminate before databases are stopped; databases must be Ready before applications return. No volumes/pods are deleted by the guard and no force termination is issued. SIGTERM grace is 120 seconds for apps and 300 seconds for databases. Databases retain their storage; no shutdown mechanism can promise immunity from power loss or underlying media failure.

Prometheus, Loki, Alertmanager, routing and Kubernetes infrastructure are outside shedding. Kubernetes system-critical priorities remain intact. Guard read-only mounts of the node /proc and cgroup root observe physical host stats and Minikube container headroom, verified against this Docker-driver installation. RBAC limits patching to the explicit application/database names and pressure-state.

CPU request shares and PriorityClass are different: this guard provides the owner's overload stop order; PriorityClass alone is not a CPU speed or RAM reservation mechanism. Resource requests remain measured application budgets.

GitHub tagged release deploys the exact released controller image and installs this policy. Rollback: pause guard deployment, inspect pressure-state, restore stopped groups through Guard.resume (databases first) before reverting the controller; preserve pressure-state until recovery is complete. CI application deployments must not override an active suspension.

Storage stalls remain under investigation: etcd data is on the root microSD ext4 filesystem, and retained logs contain slow fdatasync. Ordered shedding can reduce contention but is not proof that all disk/API stalls are repaired.
