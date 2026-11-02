# Kubernetes retrieval question inventory

The frozen machine-readable set is retained separately from source history. These are authored diagnostic questions, not user logs or measured workload frequencies. Every intent has both phrasings in all three pinned editions. The development/reserved split groups the whole intent, not individual query variants.

| Intent | Split | Keyword query | Ordinary question |
| --- | --- | --- | --- |
| pod-pending | development | `Pending` | Why is my Pod in the Pending phase? |
| crashloop | development | `CrashLoopBackOff` | Is CrashLoopBackOff a Pod phase or a container restart condition? |
| restart-policy | development | `restartPolicy OnFailure Never` | When should I use OnFailure rather than Never for container restarts? |
| restart-rules | reserved | `ContainerRestartRules` | Can I give one container a restart policy different from its Pod? |
| restart-backoff | development | `container restart backoff` | How long does Kubernetes wait before repeatedly restarting a failed container? |
| termination-default | development | `terminationGracePeriodSeconds` | By default, how much time does a Pod get to shut down before it is killed? |
| prestop-grace | reserved | `preStop grace period` | Does the preStop hook run inside the termination grace period? |
| force-delete | development | `forced Pod termination` | Does force deleting a Pod guarantee that its processes have stopped? |
| startup-probe | reserved | `startupProbe livenessProbe` | How do I stop liveness checks killing an application that starts slowly? |
| readiness-probe | development | `readinessProbe failure` | Does a failed readiness check restart the container or stop traffic? |
| init-order | development | `init containers sequential` | Do init containers run in parallel or wait for each other to finish? |
| init-failure | reserved | `init container restartPolicy Never` | What happens when an init container fails and the Pod restart policy is Never? |
| init-probes | development | `init containers probes` | Can an ordinary init container have readiness or liveness probes? |
| sidecar-job | reserved | `Jobs sidecar containers` | Can a Job finish while its sidecar container is still running? |
| sidecar-shutdown | development | `sidecar shutdown order` | In what order are sidecars stopped when a Pod terminates? |
| ephemeral-resources | development | `ephemeral containers resources` | Can I set resource requests or limits for an ephemeral debug container? |
| ephemeral-mutation | reserved | `ephemeral container change remove` | Can I edit or remove an ephemeral container after adding it? |
| downward-delivery | development | `downward API environment volume` | How can a container read its Pod metadata without calling the API server? |
| downward-resources | reserved | `resourceFieldRef cpu memory` | How can a container read CPU and memory request or limit values? |
| qos-guaranteed | development | `Guaranteed CPU memory requests limits` | What CPU and memory settings make a Pod Guaranteed QoS? |
| qos-burstable | reserved | `Burstable QoS criteria` | What makes a Pod Burstable rather than Guaranteed or BestEffort? |
| qos-besteffort | development | `BestEffort QoS` | What QoS class does a Pod get when neither the Pod nor its containers have CPU or memory requests or limits? |
| disruption-types | development | `voluntary involuntary disruptions` | Which Pod interruptions count as voluntary rather than involuntary? |
| pdb-limits | development | `PodDisruptionBudget involuntary disruptions` | Does a Pod disruption budget prevent every kind of disruption? |
| pod-hostname | development | `hostname subdomain` | How do I give a Pod a hostname and subdomain? |
| fqdn-length | reserved | `setHostnameAsFQDN 64` | Why can setting the full DNS name as a Pod hostname stop it from starting? |
| userns-root | development | `hostUsers false user namespaces` | Does root inside a Pod user namespace have root privileges on the host? |
| userns-host | development | `user namespaces hostNetwork hostPID hostIPC` | Can a Pod use user namespaces together with host networking or host process namespaces? |
| sandbox-ready | reserved | `PodReadyToStartContainers` | What does PodReadyToStartContainers tell me about a Pod sandbox and networking? |
| static-control | development | `static Pods mirror Pods` | Who manages static Pods, and what does their mirror Pod in the API server represent? |

Six controls are reported separately:
- `ConcordanceNoSuchFeatureZyxw98765`: A deliberately nonexistent technical identifier.
- `Amazon RDS point in time recovery settings`: An Amazon database restore procedure outside the selected Pod docs.
- `Kafka replication factor configuration`: Kafka replication configuration outside the selected Pod docs.
- `PostgreSQL streaming replication lag`: PostgreSQL replication operations outside the selected Pod docs.
- `Amazon S3 bucket lifecycle expiration`: Storage expiration, despite the overlapping word lifecycle.
- `Terraform remote state locking`: Terraform state operations, despite overlapping state terminology.

References are known useful evidence for each declared need; they are not an exhaustive list of correct answers. The [evaluation contract](../decisions/retrieval-evaluation.md) defines what can be reported.
