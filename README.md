# Kubernetes Event-Driven Diagnostic Operator

> An intelligent, automated diagnostic operator that monitors Kubernetes Hub Cluster events, detects spoke cluster failures, and automatically launches targeted must-gather diagnostics with log persistence for root cause analysis.

![Kubernetes](https://img.shields.io/badge/kubernetes-v1.28+-blue.svg)
![Go](https://img.shields.io/badge/go-1.21+-00ADD8.svg)
![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)

---

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
  - [Event-Driven Workflow](#event-driven-workflow)
  - [Component Architecture](#component-architecture)
  - [How It Works](#how-it-works)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
  - [Building from Source](#building-from-source)
  - [Deploying with Kubernetes Manifests](#deploying-with-kubernetes-manifests)
  - [Verification](#verification)
- [Configuration](#configuration)
  - [Operator Flags](#operator-flags)
  - [Diagnostic Rules](#diagnostic-rules)
  - [Adding Custom Rules](#adding-custom-rules)
- [Usage & Examples](#usage--examples)
  - [Example 1: ETCD Corruption Diagnostic](#example-1-etcd-corruption-diagnostic)
  - [Example 2: Network Failure Diagnostic](#example-2-network-failure-diagnostic)
  - [Example 3: Viewing Collected Logs](#example-3-viewing-collected-logs)
- [Monitoring & Observability](#monitoring--observability)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
  - [Project Structure](#project-structure)
  - [Running Tests](#running-tests)
  - [Running Locally](#running-locally)
  - [Adding New Diagnostic Rules](#adding-new-diagnostic-rules)
- [Security Considerations](#security-considerations)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

The **Kubernetes Event-Driven Diagnostic Operator** automates the detection and diagnosis of spoke/managed cluster failures in multi-cluster Kubernetes environments. Instead of manual intervention when clusters fail, this operator:

1. **Loads** pre-defined diagnostic templates mapping error patterns to must-gather images
2. **Monitors** Hub Cluster events using hub kubeconfig for `Type=Warning` events
3. **Matches** event messages against regex patterns to select the appropriate must-gather image (events that match no rule are silently skipped)
4. **Parses** event metadata and message to extract spoke/managed cluster name using a multi-strategy parser
5. **Copies** spoke/managed cluster kubeconfig from the spoke namespace into the operator namespace
6. **Spawns** lightweight Kubernetes Jobs with the matched must-gather image
7. **Executes** diagnostics on spoke cluster using mounted kubeconfig
8. **Persists** diagnostic logs to external NFS or SDS (Software Defined Storage) nodes via RWX PVC
9. **Cleans up** automatically using Kubernetes TTL-based garbage collection

This approach ensures **non-blocking operation**, **targeted diagnostics**, and **persistent log collection** without overwhelming the Hub Cluster.

### Terminology

- **Hub Cluster**: The central Kubernetes cluster running a multi-cluster management solution (e.g., RHACM) where this diagnostic operator is deployed. The Hub monitors and manages multiple spoke/managed clusters and receives their events.

- **Spoke/Managed Cluster**: Target Kubernetes clusters being monitored by the Hub. These are the clusters that may experience failures and require diagnostic data collection. Also referred to as "managed clusters" in RHACM terminology.

---

## Architecture

### Event-Driven Workflow

```mermaid
flowchart TB
    Template[Pre-defined data collection template]
    
    HubKubeconfig[Export Hub Cluster KUBECONFIG]
    EventReceiver[Cluster events receiver<br/>types=Warning]
    ImageExtractor[Data-driven image name extractor]
    OutputParser[Cluster name parser<br/>multi-strategy extraction]
    CopySecret[Copy spoke kubeconfig secret<br/>to operator namespace]
    MustGather[Run must-gather<br/>with specific image or without]
    StoreLog[Store system logs to<br/>external NFS or SDS nodes]
    GarbageCollector[System log garbage collector]
    
    Template --> ImageExtractor
    HubKubeconfig --> EventReceiver
    EventReceiver --> ImageExtractor
    ImageExtractor --> OutputParser
    OutputParser --> CopySecret
    CopySecret --> MustGather
    MustGather --> StoreLog
    StoreLog --> GarbageCollector
```

### Component Architecture

```mermaid
graph TB
    subgraph config [Configuration Layer]
        Template[Pre-defined Data Collection Template<br/>internal/config/template.go]
    end
    
    subgraph operator [Diagnostic Operator Pod]
        EventReconciler[Event Reconciler<br/>Watches Warning Events]
        ImageExtractor[Image Name Extractor<br/>Matches patterns to images]
        ClusterParser[Cluster Name Parser<br/>Multi-strategy extraction]
        SecretCopier[Secret Copier<br/>Copies kubeconfig to operator NS]
        JobCreator[Job Creator<br/>Spawns Diagnostic Jobs]
    end
    
    subgraph hubCluster [Hub Cluster Resources]
        Events[Kubernetes Events<br/>Type: Warning]
        HubKubeconfig[Hub Cluster Kubeconfig<br/>Event API Access]
        Jobs[Diagnostic Jobs<br/>must-gather executors]
    end
    
    subgraph spokeCluster [Spoke/Managed Clusters]
        SpokeKubeconfig["Spoke Cluster Kubeconfig Secrets<br/>{cluster-name}-admin-kubeconfig in spoke NS"]
    end
    
    subgraph storage [Persistent Storage]
        StorageNodes[NFS or SDS Nodes<br/>RWX PVC Log Repository]
    end
    
    HubKubeconfig --> EventReconciler
    Events --> EventReconciler
    EventReconciler --> ImageExtractor
    Template --> ImageExtractor
    ImageExtractor --> ClusterParser
    ClusterParser --> SecretCopier
    SpokeKubeconfig --> SecretCopier
    SecretCopier --> JobCreator
    JobCreator --> Jobs
    Jobs --> StorageNodes
    StorageNodes --> GC[TTL Garbage Collector]
```

### Detailed Implementation Flow

The following diagram shows the complete 9-step implementation flow as designed in the architecture:

```mermaid
flowchart TB
    step1[Step 1: Pre-defined data collection template<br/>Loaded at operator startup]
    step2[Step 2: Export Hub Cluster KUBECONFIG<br/>In-cluster service account]
    step3[Step 3: Cluster events receiver<br/>Filter: types=Warning]
    step4[Step 4: Data-driven image name extractor<br/>Match pattern to must-gather image]
    step5[Step 5: Output parser<br/>Extract cluster name via multi-strategy parser]
    step6[Step 6: Export Spoke Cluster KUBECONFIG<br/>Copy secret to operator namespace]
    step7[Step 7: Run must-gather<br/>With specific or default image]
    step8[Step 8: Store logs to NFS/SDS<br/>Persistent RWX storage]
    step9[Step 9: Garbage collector<br/>TTL-based cleanup]
    
    step1 --> step4
    step2 --> step3
    step3 --> step4
    step4 --> step5
    step5 --> step6
    step6 --> step7
    step7 --> step8
    step8 --> step9
```

### How It Works

The operator follows a 9-step implementation flow to automatically diagnose and collect logs from failing spoke/managed clusters:

#### Step 1: Pre-defined Data Collection Template

The operator loads diagnostic rules from `internal/config/template.go` at startup. These templates define regex patterns and their corresponding must-gather images.

**Code reference**: `LoadTemplates()` function in `config/template.go`

**Current rules**:
- ETCD corruption pattern → `<your-registry>/diagnostic-operator-system/ose-must-gather:latest`
- Network CNI failure pattern → `<your-registry>/diagnostic-operator-system/ose-must-gather:latest`
- Default fallback (safety net in `CreateDiagnosticJob()`) → `<your-registry>/diagnostic-operator-system/must-gather:latest`

#### Step 2: Export Hub Cluster's KUBECONFIG

The operator runs within the Hub Cluster and uses the in-cluster service account credentials to access the Kubernetes Event API.

**Configuration**: In-cluster service account with permissions to watch Events cluster-wide

**Code reference**: `ctrl.GetConfigOrDie()` in `cmd/main.go`

#### Step 3: Cluster Events Receiver (types=Warning)

The `EventReconciler` (in `internal/controller/event_watcher.go`) watches Kubernetes Event objects and filters only events where `Type == "Warning"`.

**Code reference**: `SetupWithManager()` function in `event_watcher.go`

**Filtering logic**:
```go
CreateFunc: func(e event.CreateEvent) bool {
    evt := e.Object.(*corev1.Event)
    return evt.Type == "Warning"
}
```

#### Step 4: Data-driven Image Name Extractor

**Note**: In the actual implementation, image matching runs **before** cluster name parsing. If the event message does not match any diagnostic rule, the event is silently skipped without attempting to parse a cluster name. This avoids unnecessary work for irrelevant Warning events.

Using the pre-defined template (Step 1), the extractor matches the error message against regex patterns to determine which must-gather image to use:
- ETCD errors → matched must-gather image
- Network errors → matched must-gather image
- Unmatched events → skipped (empty string returned)

**Code reference**: `determineImage()` function in `event_watcher.go`

**Matching logic**:
```go
for _, rule := range r.Rules {
    if rule.Pattern.MatchString(msg) {
        return rule.Image
    }
}
return "" // No match — event will be skipped
```

#### Step 5: Cluster Events Receiver Output Parser

After a diagnostic rule matches, the output parser extracts the spoke/managed cluster name using a **three-strategy priority parser**:

| Priority | Strategy | Source | Example |
|----------|----------|--------|---------|
| 1 (highest) | **Strategy A** | `InvolvedObject.Kind` is `ClusterDeployment` or `ManagedCluster` | Uses `InvolvedObject.Name` directly |
| 2 | **Strategy B** | Event namespace has `spoke-` prefix | Uses namespace as cluster name (e.g., `spoke-prod-1`) |
| 3 (lowest) | **Strategy C** | Regex patterns on message text | Matches `ClusterDeployment <name>`, `ManagedCluster <name>`, `cluster <name>`, `on <name>` |

If none of the strategies produce a cluster name, the event is skipped.

**Code reference**: `parseClusterName()` function in `event_watcher.go`

**Example parsing**:
```
Strategy A: Event with InvolvedObject Kind=ClusterDeployment, Name=spoke-prod-1
            → cluster name = "spoke-prod-1"

Strategy B: Event in namespace "spoke-dev-2" (no ClusterDeployment/ManagedCluster kind)
            → cluster name = "spoke-dev-2"

Strategy C: Message "etcd database corruption detected on cluster spoke-prod-1"
            → cluster name = "spoke-prod-1"
```

#### Step 6: Export Spoke/Managed Cluster's KUBECONFIG

The operator retrieves the spoke cluster's kubeconfig from a Kubernetes Secret and copies it into the operator namespace. Kubernetes does not support cross-namespace volume mounts, so this copy step is required for the diagnostic Job pod to mount the kubeconfig.

**Code reference**: `copySecretToNamespace()` function in `job_creator.go`

**Secret flow**:
1. **Source secret**: `{cluster-name}-admin-kubeconfig` in namespace `{cluster-name}` (the spoke cluster's own namespace on the hub)
2. **Copy**: The operator creates an idempotent copy named `diag-kubeconfig-{cluster-name}` in `diagnostic-operator-system`
3. **Mount**: The diagnostic Job pod mounts the copied secret

**Source secret format**:
- Name: `{cluster-name}-admin-kubeconfig` (e.g., `spoke-prod-1-admin-kubeconfig`)
- Namespace: `{cluster-name}` (e.g., `spoke-prod-1`)
- Key: `kubeconfig`

**Copied secret format**:
- Name: `diag-kubeconfig-{cluster-name}` (e.g., `diag-kubeconfig-spoke-prod-1`)
- Namespace: `diagnostic-operator-system`
- Labels: `app.kubernetes.io/managed-by: diagnostic-operator`, `diagnostic-operator/cluster: {cluster-name}`

#### Step 7: Run Diagnostic Collection with Specific Image or Without

A Kubernetes Job is created that:
- Uses the image determined in Step 4
- Mounts the spoke cluster kubeconfig from Step 6
- Runs the diagnostic collection command against the spoke cluster
- Operates independently without blocking the operator
- Has TTL set for automatic cleanup

**Code reference**: `CreateDiagnosticJob()` function in `job_creator.go`

**Job specifications**:
- Name prefix: `diag-{cluster-name}-` (generated via `GenerateName`)
- Container name: `must-gather-executor`
- ServiceAccount: `diagnostic-job-sa`
- RestartPolicy: `OnFailure`
- Command: `["/usr/bin/diagnostic-collector"]` (configurable diagnostic binary)
- Args: `["collect", "--dest-dir=/mnt/nfs/logs/{cluster-name}"]`
- Environment: `KUBECONFIG=/etc/secret/kubeconfig`

#### Step 8: Store System Logs to External NFS or SDS Nodes

The Job pod mounts a ReadWriteMany (RWX) PersistentVolumeClaim backed by either:
- **NFS** (Network File System) - Traditional shared storage
- **SDS** (Software Defined Storage) - Ceph, Portworx, OpenEBS, OpenShift Data Foundation (OpenShift-specific)

Logs are written to `/mnt/nfs/logs/{cluster-name}/` ensuring persistence beyond pod lifetime and accessibility for offline root cause analysis.

**Code reference**: PVC volume and volume mount definitions in `CreateDiagnosticJob()` in `job_creator.go`

**Storage configuration**:
- PVC name: `logs-pvc`
- Access mode: ReadWriteMany (RWX)
- Mount path: `/mnt/nfs`
- Destination: `/mnt/nfs/logs/{cluster-name}/`

#### Step 9: System Log Garbage Collector

After the diagnostic Job completes (successfully or with failure), the Kubernetes TTL (Time To Live) controller automatically deletes the Job and its pods after the configured period (default: 3600 seconds / 1 hour).

**Code reference**: `TTLSecondsAfterFinished` field in `CreateDiagnosticJob()` in `job_creator.go`

**Cleanup behavior**:
- Successful jobs: Deleted after TTL expires
- Failed jobs: Also deleted after TTL expires (logs are already persisted)
- TTL default: 3600 seconds (1 hour)
- Customizable per deployment needs

---

## Features

- **Event-Driven Architecture**: Real-time monitoring of Hub Cluster events for `Type=Warning`
- **Smart Pattern Matching**: Regex-based template engine maps specific error patterns to specialized diagnostic images
- **Non-Blocking Job Offloading**: Spawns lightweight Kubernetes Jobs to run diagnostics asynchronously
- **Persistent Log Storage**: Mounts RWX (ReadWriteMany) storage to collect and persist logs outside ephemeral pods
- **Automatic Garbage Collection**: Uses Kubernetes TTL to automatically clean up diagnostic jobs after completion
- **Targeted Diagnostics**: Different must-gather images for different failure types (ETCD, Network, Storage, etc.)
- **Leader Election Support**: Enables high-availability deployments with multiple replicas
- **Health & Readiness Probes**: Built-in health checks for production deployments

---

## Prerequisites

| Requirement | Version/Details |
|------------|-----------------|
| **Kubernetes** | v1.28+ (based on API compatibility) |
| **Go** | 1.21+ (for building from source) |
| **Container Runtime** | Docker, Podman, or CRI-O |
| **Shared Storage** | RWX StorageClass (NFS or SDS - Software Defined Storage) |
| **Spoke/Managed Cluster Credentials** | Kubeconfig secrets named `{cluster-name}-admin-kubeconfig` in namespace `{cluster-name}` |

**Important**: The operator requires a **ReadWriteMany (RWX)** StorageClass to allow multiple diagnostic jobs to write logs simultaneously. Common options include:
- **NFS**: `managed-nfs-storage` or custom NFS provisioners
- **SDS (Software Defined Storage)**: 
  - `ocs-storagecluster-cephfs` (OpenShift Data Foundation / Ceph, OpenShift-specific)
  - Portworx shared volumes
  - OpenEBS NFS provisioner
  - Other distributed storage systems

---

## Installation

### Building from Source

1. **Clone the repository**:
```bash
git clone https://github.com/username/event-driven-diagnostic-operator.git
cd event-driven-diagnostic-operator
```

2. **Build the container image**:
```bash
# Using Podman (recommended for container builds)
podman build -t <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.4 -f Containerfile .

# Or using Docker
docker build -t <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.4 -f Containerfile .
```

3. **Push to your container registry**:
```bash
podman push <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.4
```

### Deploying with Kubernetes Manifests

1. **Create the namespace**:
```bash
kubectl create namespace diagnostic-operator-system
```

2. **Create the ServiceAccount and RBAC**:
```bash
kubectl apply -f deploy/rbac.yaml
```

This creates all required RBAC resources:
- `diagnostic-operator-sa`: ServiceAccount for the operator
- `diagnostic-job-sa`: ServiceAccount for diagnostic jobs
- `diagnostic-operator-role`: ClusterRole with permissions for events (`get/list/watch/create/patch`), secrets (`get/list/watch`), and jobs (`create/get/list/watch/delete`)
- `diagnostic-operator-binding`: ClusterRoleBinding for cluster-wide event access
- `diagnostic-operator-leader-election`: Role + RoleBinding for leader election leases
- `diagnostic-operator-secrets`: Role + RoleBinding for creating/deleting copied kubeconfig secrets in the operator namespace
- `diagnostic-job-nfs-role`: Role + RoleBinding granting SCC `privileged` access for NFS mounting (OpenShift-specific)

3. **Create the PersistentVolumeClaim for log storage**:
```bash
kubectl apply -f deploy/pvc.yaml
```

**Important**: Edit `deploy/pvc.yaml` to set your StorageClass:
```yaml
spec:
  storageClassName: managed-nfs-storage  # Change to your RWX storage class
```

Find available StorageClasses:
```bash
kubectl get storageclass
```

4. **Deploy the operator**:

Edit `deploy/deployment.yaml` to set your image. The default uses your container registry:
```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        image: <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.4  # Update this
```

Then apply:
```bash
kubectl apply -f deploy/deployment.yaml
```

### Verification

Check that the operator is running:
```bash
kubectl get pods -n diagnostic-operator-system
```

Expected output:
```
NAME                                   READY   STATUS    RESTARTS   AGE
diagnostic-operator-7d8f9c8b5d-abcde   1/1     Running   0          30s
```

Check operator logs:
```bash
kubectl logs -n diagnostic-operator-system deployment/diagnostic-operator -f
```

You should see:
```
INFO    setup    Loaded diagnostic rules    {"count": 2}
INFO    setup    starting manager
```

---

## Configuration

### Operator Flags

The operator supports the following command-line flags (defined in `cmd/main.go`):

| Flag | Default | Description |
|------|---------|-------------|
| `--health-probe-bind-address` | `:8081` | Address for health and readiness probes |
| `--leader-elect` | `false` | Enable leader election for HA deployments |
| `--zap-devel` | `true` | Enable development logging (verbose) |
| `--zap-encoder` | `json` | Log encoding format (`json` or `console`) |

**Note**: The deployment manifest also sets a `LOG_RETENTION_HOURS` environment variable (default `24`). This is reserved for future use and is not currently consumed by the Go code.

**Example**: Enable leader election and production logging:
```yaml
# In deploy/deployment.yaml
spec:
  containers:
  - name: manager
    command:
    - /manager
    - --leader-elect=true
    - --zap-devel=false
    - --zap-encoder=json
```

### Diagnostic Rules

Diagnostic rules are defined in `internal/config/template.go`. Each rule maps an error pattern to a specific must-gather image.

**Current default rules**:

| Rule Name | Pattern | Must-Gather Image |
|-----------|---------|-------------------|
| ETCD Corruption | `(?i)etcd.*database.*corruption` | `<your-registry>/diagnostic-operator-system/ose-must-gather:latest` |
| OVN Network Failure | `(?i)Network.*CNI.*failed` | `<your-registry>/diagnostic-operator-system/ose-must-gather:latest` |
| Default (fallback) | N/A | `<your-registry>/diagnostic-operator-system/must-gather:latest` |

The Default fallback is a safety net hardcoded in `CreateDiagnosticJob()`. It is used when the `image` parameter is empty, but in practice this path is currently unreachable because the reconciler skips events that match no rule before the job creator is invoked.

**Pattern matching is case-insensitive** (`(?i)` flag) and uses Go's `regexp` package.

### Ignored Warning Events

The operator intentionally ignores certain Warning event patterns to avoid unnecessary diagnostic overhead:

**Managed Cluster Connection Check Failures**

- **Pattern**: `connection check from the managed cluster to the hub cluster.*failed`
- **Reason**: `AvailableUnknown`
- **Object Type**: `ManagedCluster`
- **Rationale**: These events are typically transient and self-healing. RHACM continuously retries connection checks, and running diagnostics on every connection blip would create unnecessary cluster load. Connection issues are best diagnosed through network policy reviews, firewall rules, and DNS configuration rather than cluster-wide must-gather data collection.

**Example of ignored event**:

```yaml
# This event will NOT trigger diagnostics
apiVersion: v1
kind: Event
type: Warning
reason: AvailableUnknown
message: "ManagedCluster target-spoke-cluster is successfully imported. However, the connection check from the managed cluster to the hub cluster has failed"
involvedObject:
  kind: ManagedCluster
  name: target-spoke-cluster
```

See `examples/ignored-managed-cluster-connection-event.yaml` for a complete example.

**Note**: If you need to diagnose persistent connection issues, manually trigger a network must-gather on the affected spoke cluster using the targeted diagnostic approach rather than relying on automatic event-driven diagnostics.

### Adding Custom Rules

To add new diagnostic rules:

1. **Edit** `internal/config/template.go`:

```go
func LoadTemplates() []DiagnosticRule {
    return []DiagnosticRule{
        {
            Name:    "ETCD Corruption",
            Pattern: regexp.MustCompile(`(?i)etcd.*database.*corruption`),
            Image:   "<your-registry>/diagnostic-operator-system/ose-must-gather:latest",
        },
        {
            Name:    "OVN Network Failure",
            Pattern: regexp.MustCompile(`(?i)Network.*CNI.*failed`),
            Image:   "<your-registry>/diagnostic-operator-system/ose-must-gather:latest",
        },
        // ADD YOUR NEW RULE HERE
        {
            Name:    "Storage Provisioning Failure",
            Pattern: regexp.MustCompile(`(?i)StorageClass.*provision.*failed`),
            Image:   "<your-registry>/diagnostic-operator-system/ose-must-gather:latest",
        },
    }
}
```

2. **Rebuild** the operator image:
```bash
podman build -t <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.5 -f Containerfile .
podman push <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.5
```

3. **Update** the deployment:
```bash
kubectl set image deployment/diagnostic-operator \
  manager=<your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.5 \
  -n diagnostic-operator-system
```

**Testing regex patterns**:
Use Go's regex tester or online tools like [regex101.com](https://regex101.com/) with flavor set to "Golang".

---

## Usage & Examples

### Example 1: ETCD Corruption Diagnostic

Create a Warning event that simulates an ETCD corruption:

```yaml
apiVersion: v1
kind: Event
metadata:
  name: test-etcd-failure
  namespace: default
type: Warning
reason: ETCDCorruption
message: "ClusterDeployment spoke-prod-1 etcd database corruption detected"
involvedObject:
  kind: ClusterDeployment
  name: spoke-prod-1
```

Apply the event:
```bash
kubectl apply -f test-etcd-event.yaml
```

**Expected outcome**:
1. Operator detects the Warning event
2. Matches pattern: `(?i)etcd.*database.*corruption` (image matching runs first)
3. Extracts cluster name: `spoke-prod-1` (via Strategy A: InvolvedObject kind `ClusterDeployment`)
4. Copies kubeconfig secret from `spoke-prod-1` namespace to operator namespace
5. Creates Job `diag-spoke-prod-1-<random>` using the matched must-gather image
6. Job writes logs to: `/mnt/nfs/logs/spoke-prod-1/`

Verify the job was created:
```bash
kubectl get jobs -n diagnostic-operator-system
```

### Example 2: Network Failure Diagnostic

Simulate a CNI network failure:

```yaml
apiVersion: v1
kind: Event
metadata:
  name: test-network-failure
  namespace: default
type: Warning
reason: NetworkFailure
message: "ClusterDeployment spoke-dev-2 Network CNI plugin failed to initialize"
involvedObject:
  kind: ClusterDeployment
  name: spoke-dev-2
```

**Expected outcome**:
- Matches pattern `(?i)Network.*CNI.*failed`
- Extracts cluster name `spoke-dev-2` via Strategy A (InvolvedObject kind `ClusterDeployment`)
- Creates Job `diag-spoke-dev-2-<random>` with the matched must-gather image
- Logs written to: `/mnt/nfs/logs/spoke-dev-2/`

### Example 3: Viewing Collected Logs

Access the NFS storage to view diagnostic logs:

**Option 1**: From within the cluster:
```bash
# Create a debug pod with NFS mount
kubectl run nfs-viewer -n diagnostic-operator-system --image=busybox --restart=Always --overrides='
{
  "spec": {
    "containers": [{
      "name": "nfs-viewer",
      "image": "busybox",
      "command": ["sh", "-c", "sleep infinity"],
      "volumeMounts": [{
        "name": "logs",
        "mountPath": "/logs"
      }]
    }],
    "volumes": [{
      "name": "logs",
      "persistentVolumeClaim": {
        "claimName": "logs-pvc"
      }
    }]
  }
}'

# Inside the pod
ls /logs/
# Output: spoke-prod-1/  spoke-dev-2/

ls /logs/spoke-prod-1/
# Output: must-gather.tar.gz  timestamp  ...
```

**Option 2**: From the NFS server directly:
```bash
# SSH to your NFS server
ssh nfs-server.example.com

# Navigate to the exported path
cd /exports/logs
ls -lh
```

**Option 3**: Copy logs to local machine:
```bash
# Get a running operator pod name
POD=$(kubectl get pod -n diagnostic-operator-system -l app=diagnostic-operator -o jsonpath='{.items[0].metadata.name}')

# Copy logs from PVC via operator pod
kubectl exec -n diagnostic-operator-system $POD -- tar czf - /mnt/nfs/logs/spoke-prod-1 | tar xzf -
```

---

## Monitoring & Observability

### Health & Readiness Checks

The operator exposes health and readiness endpoints (configured in `cmd/main.go`):

| Endpoint | Port | Purpose |
|----------|------|---------|
| `/healthz` | 8081 | Liveness probe - checks if operator is alive |
| `/readyz` | 8081 | Readiness probe - checks if operator is ready to serve |

**Kubernetes probe configuration**:
```yaml
# Add to deploy/deployment.yaml
spec:
  containers:
  - name: manager
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8081
      initialDelaySeconds: 15
      periodSeconds: 20
    readinessProbe:
      httpGet:
        path: /readyz
        port: 8081
      initialDelaySeconds: 5
      periodSeconds: 10
```

### Leader Election Status

When leader election is enabled (`--leader-elect=true`), only one replica actively reconciles events. Check leader election status:

```bash
kubectl get lease -n diagnostic-operator-system
kubectl describe lease diagnostic-operator.example.com -n diagnostic-operator-system
```

### Metrics to Monitor

While the operator doesn't expose Prometheus metrics by default, monitor these Kubernetes metrics:

1. **Events Processed**: Count of reconcile loops
```bash
kubectl logs -n diagnostic-operator-system deployment/diagnostic-operator | grep "Reconcile"
```

2. **Jobs Created**: Number of diagnostic jobs
```bash
kubectl get jobs -n diagnostic-operator-system --show-labels
```

3. **Job Success/Failure Rate**:
```bash
kubectl get jobs -n diagnostic-operator-system -o json | \
  jq '.items[] | select(.status.conditions[].type=="Complete") | .metadata.name'
```

4. **Storage Usage**:
```bash
kubectl exec -n diagnostic-operator-system deployment/diagnostic-operator -- \
  df -h /mnt/nfs
```

5. **Job Cleanup Status** (TTL controller):
```bash
kubectl get jobs -n diagnostic-operator-system -o wide
# Jobs should auto-delete 1 hour after completion
```

### Logging

View operator logs:
```bash
kubectl logs -n diagnostic-operator-system deployment/diagnostic-operator -f
```

Log levels can be adjusted with zap flags:
- Development mode: `--zap-devel=true` (verbose, includes stack traces)
- Production mode: `--zap-devel=false` (structured JSON logs)

### Manual Event Queries

While the operator automatically watches for Warning events, you can also manually query events using kubectl commands or the operator's query API.

#### Using kubectl Commands

**Query Warning events in a specific spoke/managed cluster namespace**:

```bash
# Using hub cluster kubeconfig
export KUBECONFIG=/path/to/hub-cluster-kubeconfig

# Query Warning events in a spoke cluster namespace
kubectl get events -n target-spoke-cluster --field-selector type=Warning

# Get detailed output with timestamps
kubectl get events -n target-spoke-cluster --field-selector type=Warning -o wide

# Get JSON output for programmatic parsing
kubectl get events -n target-spoke-cluster --field-selector type=Warning -o json
```

**Query Warning events across all namespaces**:

```bash
# See all Warning events on the hub cluster
kubectl get events --all-namespaces --field-selector type=Warning

# Filter for managed cluster related events
kubectl get events --all-namespaces --field-selector type=Warning | grep ManagedCluster

# Sort by last seen timestamp
kubectl get events --all-namespaces --field-selector type=Warning --sort-by='.lastTimestamp'
```

#### Using the Operator's Query API

The operator exposes HTTP endpoints on port 8082 for programmatic event queries:

**Query events in a specific namespace**:

```bash
# Port-forward to access the API
kubectl port-forward -n diagnostic-operator-system deployment/diagnostic-operator 8082:8082

# Query events in a specific namespace
curl http://localhost:8082/query-events?namespace=target-spoke-cluster

# Query all namespaces
curl http://localhost:8082/query-events-all
```

**From inside a pod in the cluster**:

```bash
kubectl exec -n diagnostic-operator-system deployment/diagnostic-operator -- \
  curl http://localhost:8082/query-events?namespace=target-spoke-cluster
```

#### Common Event Query Patterns

```bash
# Check for connection failures (ignored by operator)
kubectl get events -n target-spoke-cluster \
  --field-selector type=Warning,reason=AvailableUnknown

# Check for ETCD related warnings
kubectl get events --all-namespaces \
  --field-selector type=Warning \
  -o json | jq '.items[] | select(.message | contains("etcd"))'

# Check for managed cluster events
kubectl get events --all-namespaces \
  --field-selector type=Warning,involvedObject.kind=ManagedCluster

# Get events from the last hour
kubectl get events --all-namespaces \
  --field-selector type=Warning \
  --sort-by='.lastTimestamp' | tail -20
```

#### Understanding Event Output

```
LAST SEEN   TYPE      REASON             OBJECT                      MESSAGE
23m         Warning   AvailableUnknown   managedcluster/cluster-1    The cluster-1 is successfully...
```

- **LAST SEEN**: How long ago the event was last observed
- **TYPE**: Event severity (Warning, Normal, Error)
- **REASON**: Short machine-readable reason code
- **OBJECT**: Kubernetes object that triggered the event
- **MESSAGE**: Human-readable description

#### Query API Response Format

```json
{
  "namespace": "target-spoke-cluster",
  "count": 2,
  "events": [
    {
      "Namespace": "target-spoke-cluster",
      "Name": "example-event.abc123",
      "Type": "Warning",
      "Reason": "AvailableUnknown",
      "Message": "Connection check failed",
      "LastSeen": "2026-02-13T17:23:00Z",
      "ObjectKind": "ManagedCluster",
      "ObjectName": "target-spoke-cluster"
    }
  ]
}
```

---

## Troubleshooting

### Issue #1: Events Not Triggering Jobs

**Symptoms**: Warning events are created but no diagnostic jobs appear.

**Diagnosis**:
```bash
# Check operator logs for reconcile events
kubectl logs -n diagnostic-operator-system deployment/diagnostic-operator | grep "Reconcile"

# Verify events are visible to the operator
kubectl get events --all-namespaces --field-selector type=Warning
```

**Common causes**:
1. **Event type mismatch**: Ensure event has `type: Warning` (case-sensitive)
2. **No diagnostic rule matched**: The `determineImage()` function must match the event message against a regex rule before cluster name parsing is attempted. Check that the event message matches one of the patterns in `internal/config/template.go`.
3. **Cluster name not parsed**: The `parseClusterName()` function uses three strategies in priority order:
   - **Strategy A**: `InvolvedObject.Kind` is `ClusterDeployment` or `ManagedCluster` → uses `InvolvedObject.Name`
   - **Strategy B**: Event namespace starts with `spoke-` → uses namespace as cluster name
   - **Strategy C**: Regex on message text (looks for `ClusterDeployment`, `ManagedCluster`, `cluster`, `on` keywords)
   - If none match, the event is skipped
4. **RBAC permissions**: Verify operator has permission to watch Events
```bash
kubectl auth can-i watch events --as=system:serviceaccount:diagnostic-operator-system:diagnostic-operator-sa
```

**Fix**:
Ensure the event either has an appropriate `InvolvedObject` kind, originates from a `spoke-*` namespace, or includes a cluster identifier in the message:
```yaml
# Option A: Use InvolvedObject (preferred)
involvedObject:
  kind: ClusterDeployment  # or ManagedCluster
  name: spoke-prod-1

# Option B: Use spoke-* namespace
namespace: spoke-prod-1

# Option C: Include keyword in message
message: "ClusterDeployment spoke-prod-1 <error description>"
```

### Issue #2: Jobs Fail to Start

**Symptoms**: Jobs are created but pods remain in `Pending` or `Error` state.

**Diagnosis**:
```bash
# List jobs
kubectl get jobs -n diagnostic-operator-system

# Check job details
kubectl describe job <job-name> -n diagnostic-operator-system

# Check pod status
kubectl get pods -n diagnostic-operator-system
kubectl describe pod <pod-name> -n diagnostic-operator-system
```

**Common causes**:

1. **ServiceAccount missing**: Job requires `diagnostic-job-sa`
```bash
kubectl get serviceaccount diagnostic-job-sa -n diagnostic-operator-system
```
**Fix**: Apply `deploy/rbac.yaml`

2. **Kubeconfig secret not found**: The operator reads the source secret `{cluster-name}-admin-kubeconfig` from namespace `{cluster-name}` (the spoke cluster namespace), then copies it to the operator namespace as `diag-kubeconfig-{cluster-name}`
```bash
# Check if source secret exists in the spoke namespace
kubectl get secret spoke-prod-1-admin-kubeconfig -n spoke-prod-1

# Check if the copied secret exists in the operator namespace
kubectl get secret diag-kubeconfig-spoke-prod-1 -n diagnostic-operator-system
```
**Fix**: Create the source secret in the spoke cluster's namespace:
```bash
kubectl create secret generic spoke-prod-1-admin-kubeconfig \
  --from-file=kubeconfig=/path/to/spoke-cluster-kubeconfig \
  -n spoke-prod-1
```

3. **PVC not bound**: Logs PVC must be in `Bound` state
```bash
kubectl get pvc logs-pvc -n diagnostic-operator-system
```
**Fix**: See Issue #3 below

4. **Image pull errors**: Unable to pull must-gather image
```bash
kubectl describe pod <pod-name> -n diagnostic-operator-system | grep -A5 "Events:"
```
**Fix**: Verify image exists and add imagePullSecrets if using private registry

### Issue #3: PVC Remains in Pending State

**Symptoms**: `logs-pvc` PersistentVolumeClaim stuck in `Pending`.

**Diagnosis**:
```bash
kubectl describe pvc logs-pvc -n diagnostic-operator-system
```

**Common causes**:
1. **StorageClass not found**: The `storageClassName` in `deploy/pvc.yaml` doesn't match cluster
```bash
kubectl get storageclass
```
**Fix**: Edit `deploy/pvc.yaml` and set a valid StorageClass:
```yaml
spec:
  storageClassName: ocs-storagecluster-cephfs  # Use your cluster's RWX class
```

2. **No RWX provisioner**: Cluster lacks ReadWriteMany storage
```bash
kubectl get storageclass -o custom-columns=NAME:.metadata.name,MODES:.allowedTopologies
```
**Fix**: Install NFS provisioner or SDS solution:
- NFS: Deploy NFS provisioner or use external NFS server
- SDS: Install Ceph (OpenShift Data Foundation, OpenShift-specific), Portworx, or OpenEBS
- Cloud: Use cloud provider's shared file storage (EFS, Azure Files, etc.)

3. **Insufficient storage**: Not enough capacity in storage backend
**Fix**: Reduce PVC size or add storage capacity

### Issue #4: Logs Not Persisting

**Symptoms**: Jobs complete successfully but logs are missing from storage.

**Diagnosis**:
```bash
# Check if NFS is mounted in job pod
kubectl exec -n diagnostic-operator-system <job-pod-name> -- df -h /mnt/nfs

# Check directory permissions
kubectl exec -n diagnostic-operator-system <job-pod-name> -- ls -la /mnt/nfs/logs/
```

**Common causes**:
1. **Mount point permissions**: NFS export has restrictive permissions
**Fix**: On NFS server, ensure export allows writes:
```bash
# On NFS server
chmod 777 /exports/logs
chown nfsnobody:nfsnobody /exports/logs
```

2. **Must-gather fails**: The must-gather command itself fails
```bash
# Check job logs
kubectl logs -n diagnostic-operator-system <job-pod-name>
```
**Fix**: Verify spoke/managed cluster kubeconfig is valid and has admin permissions

3. **Path mismatch**: Logs written to wrong directory
- Job writes to: `/mnt/nfs/logs/{cluster-name}/`
- Verify in `CreateDiagnosticJob()` in `job_creator.go`: `--dest-dir=/mnt/nfs/logs/` + clusterName

### Issue #5: Jobs Not Cleaning Up

**Symptoms**: Completed jobs remain indefinitely instead of auto-deleting.

**Diagnosis**:
```bash
kubectl get jobs -n diagnostic-operator-system
# Jobs should disappear ~1 hour after completion
```

**Common causes**:
1. **TTL controller disabled**: Kubernetes cluster must have TTL controller enabled
```bash
kubectl get pods -n kube-system | grep ttl
```
**Fix**: Enable TTL controller in cluster (varies by distribution)

2. **TTL not set**: Job doesn't have `TTLSecondsAfterFinished`
```bash
kubectl get job <job-name> -n diagnostic-operator-system -o yaml | grep ttlSecondsAfterFinished
```
**Fix**: This should be automatic (see `TTLSecondsAfterFinished` in `CreateDiagnosticJob()`). If missing, check operator version.

3. **Job still running**: TTL only applies to completed/failed jobs
```bash
kubectl describe job <job-name> -n diagnostic-operator-system
```

**Manual cleanup**:
```bash
# Delete old jobs manually
kubectl delete jobs -n diagnostic-operator-system --field-selector status.successful=1
```

### Issue #6: Permission Denied on NFS Mount

> **Note**: This issue is specific to OpenShift deployments using Security Context Constraints (SCC).

**Symptoms**: Pods fail with "Permission denied" when writing to NFS.

**Cause**: Security Context Constraints (SCC) restrict NFS mounts.

**Fix**: The `deploy/rbac.yaml` already includes `diagnostic-job-nfs-role` granting `privileged` SCC access to `diagnostic-job-sa`. Ensure it has been applied:
```bash
kubectl apply -f deploy/rbac.yaml
```

Alternatively, grant SCC directly via the `kubectl` CLI:
```bash
kubectl adm policy add-scc-to-user privileged -z diagnostic-job-sa -n diagnostic-operator-system

# Or use less privileged hostmount-anyuid if sufficient
kubectl adm policy add-scc-to-user hostmount-anyuid -z diagnostic-job-sa -n diagnostic-operator-system
```

---

## Development

### Project Structure

```
event-driven-diagnostic-operator/
├── cmd/
│   └── main.go                          # Entrypoint: controller-runtime Manager, query API server
├── internal/
│   ├── config/
│   │   └── template.go                  # Diagnostic rule definitions (regex → image mappings)
│   ├── controller/
│   │   ├── event_watcher.go             # EventReconciler: watches Events, filters, parses, dispatches
│   │   ├── event_watcher_test.go        # Unit tests for parseClusterName (9 table-driven cases)
│   │   ├── job_creator.go               # Secret copy + diagnostic Job creation with NFS and TTL
│   │   └── query_handler.go             # HTTP handlers for /query-events and /query-events-all
│   └── utils/
│       └── kubectl.go                   # kubectl-based event query helpers (shells out to kubectl)
├── deploy/
│   ├── rbac.yaml                        # ServiceAccounts, ClusterRole, Roles, Bindings, SCC
│   ├── pvc.yaml                         # PersistentVolumeClaim for log storage (RWX)
│   └── deployment.yaml                  # Operator Deployment manifest
├── examples/
│   └── ignored-managed-cluster-connection-event.yaml  # Sample ignored RHACM connection event
├── Containerfile                        # Multi-stage build (Go build → UBI minimal + kubectl)
├── go.mod                               # Go module dependencies
└── README.md                            # This file
```

### Running Tests

The project includes unit tests for the cluster name parsing logic:

```bash
go test ./internal/controller/ -v
```

**Test coverage**: `event_watcher_test.go` contains 9 table-driven test cases for `parseClusterName()` covering:
- Strategy A: `ClusterDeployment` and `ManagedCluster` InvolvedObject kinds
- Strategy B: `spoke-*` namespace prefix heuristic
- Strategy C: Regex extraction from message text (`ClusterDeployment`, `on cluster`, `on` keywords)
- No-match scenarios (returns empty string)
- Priority ordering (Strategy A > Strategy B > Strategy C)

### Running Locally

To run the operator code locally (outside the cluster) while connecting to your current kubecontext:

1. **Ensure kubeconfig is set**:
```bash
export KUBECONFIG=~/.kube/config
kubectl cluster-info
```

2. **Run the operator**:
```bash
go run cmd/main.go
```

The operator will:
- Connect to your current cluster
- Start watching Events
- Create jobs in the cluster when Warning events are detected

**Note**: You still need the PVC and RBAC resources deployed in the cluster.

### Testing with Simulated Events

Create test Warning events to trigger diagnostics:

```bash
# Create ETCD failure event
kubectl apply -f - <<EOF
apiVersion: v1
kind: Event
metadata:
  name: test-etcd-$(date +%s)
  namespace: default
type: Warning
reason: ETCDCorruption
message: "ClusterDeployment test-cluster-1 etcd database corruption detected"
involvedObject:
  kind: ClusterDeployment
  name: test-cluster-1
EOF

# Watch operator logs
kubectl logs -f -n diagnostic-operator-system deployment/diagnostic-operator

# Verify job creation
kubectl get jobs -n diagnostic-operator-system
```

### Adding New Diagnostic Rules

1. **Identify the error pattern**: Look at actual Warning event messages from your cluster
```bash
kubectl get events --all-namespaces --field-selector type=Warning
```

2. **Write regex pattern**: Create a pattern that uniquely identifies the error
```go
Pattern: regexp.MustCompile(`(?i)keyword1.*keyword2.*specific-error`),
```

3. **Find appropriate must-gather image**: Use your internal registry or Red Hat's public registry:
- General: `<your-registry>/diagnostic-operator-system/must-gather:latest`
- Internal registry: `<your-registry>/diagnostic-operator-system/ose-must-gather:latest`
- Custom: Build your own must-gather image

4. **Add rule to template.go**:
```go
{
    Name:    "Your Error Type",
    Pattern: regexp.MustCompile(`(?i)your.*regex.*pattern`),
    Image:   "registry/username/your-must-gather:latest",
},
```

5. **Test the pattern**: Use Go's testing framework
```go
// internal/config/template_test.go
func TestPatternMatching(t *testing.T) {
    rules := LoadTemplates()
    message := "ClusterDeployment test your actual error message"
    
    for _, rule := range rules {
        if rule.Pattern.MatchString(message) {
            t.Logf("Matched rule: %s with image: %s", rule.Name, rule.Image)
            break
        }
    }
}
```

6. **Rebuild and deploy**:
```bash
podman build -t <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.5 -f Containerfile .
podman push <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.5
kubectl set image deployment/diagnostic-operator manager=<your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.5 -n diagnostic-operator-system
```

### Debugging Tips

1. **Enable verbose logging**:
```bash
# In deploy/deployment.yaml, add flag
args: ["--zap-devel=true", "--zap-log-level=2"]
```

2. **Check controller-runtime events**:
```bash
kubectl get events -n diagnostic-operator-system --sort-by='.lastTimestamp'
```

3. **Inspect reconcile loops**:
```bash
kubectl logs -n diagnostic-operator-system deployment/diagnostic-operator | grep "Reconcile"
```

4. **Test regex patterns online**: Use [regex101.com](https://regex101.com/) with flavor "Golang"

5. **Check job pod logs**:
```bash
# Get most recent job
JOB=$(kubectl get jobs -n diagnostic-operator-system --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1].metadata.name}')

# Get pod from job
POD=$(kubectl get pods -n diagnostic-operator-system -l job-name=$JOB -o jsonpath='{.items[0].metadata.name}')

# View logs
kubectl logs -n diagnostic-operator-system $POD
```

---

## Security Considerations

### ServiceAccount Permissions

The operator uses two ServiceAccounts:

1. **diagnostic-operator-sa** (operator itself):
   - ClusterRole with permissions to:
     - `get`, `list`, `watch`, `create`, `patch` Events (cluster-wide)
     - `create`, `get`, `list`, `watch`, `delete` Jobs (namespaced)
     - `get`, `list`, `watch` Secrets (cluster-wide, for reading spoke kubeconfigs)
   - Namespace-scoped Roles for:
     - Leader election: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete` Leases
     - Secret management: `create`, `get`, `delete` Secrets (for kubeconfig copies in operator namespace)

2. **diagnostic-job-sa** (diagnostic jobs):
   - Role with permissions to:
     - `use` SecurityContextConstraints for NFS mounting (OpenShift only)

**Principle of least privilege**: Both ServiceAccounts have minimal permissions required for their function.

### Spoke/Managed Cluster Credentials

Spoke/managed cluster kubeconfigs are stored as Secrets in the Hub Cluster. The operator reads them from the spoke cluster's own namespace (e.g., `spoke-prod-1`) and copies them to the operator namespace for Job pod mounting. Best practices:

1. **Namespace isolation**: Source secrets live in spoke cluster namespaces; copies are created in `diagnostic-operator-system` with labels for tracking
2. **Access control**: Only the operator ServiceAccount can read spoke kubeconfig secrets and create copies
3. **Rotation**: Regularly rotate spoke/managed cluster credentials
4. **Audit**: Enable audit logging for Secret access

```bash
# Create spoke/managed cluster kubeconfig secret in the spoke namespace
kubectl create secret generic spoke-prod-1-admin-kubeconfig \
  --from-file=kubeconfig=/path/to/spoke-kubeconfig \
  -n spoke-prod-1

# Verify secret is not readable by default ServiceAccount
kubectl auth can-i get secret spoke-prod-1-admin-kubeconfig \
  --as=system:serviceaccount:diagnostic-operator-system:default \
  -n spoke-prod-1
# Should return "no"
```

### Network Policies

Restrict network access for the operator and diagnostic jobs:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: diagnostic-operator-netpol
  namespace: diagnostic-operator-system
spec:
  podSelector:
    matchLabels:
      app: diagnostic-operator
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring  # Allow Prometheus scraping
    ports:
    - protocol: TCP
      port: 8081  # Health checks
  egress:
  - to:
    - namespaceSelector: {}  # Allow access to Kubernetes API
  - to:
    - podSelector: {}  # Allow NFS access
    ports:
    - protocol: TCP
      port: 2049  # NFS
```

### Pod Security Standards

The operator follows Pod Security Standards (PSS):

- **Runs as non-root user**: UID 65532 (defined in Containerfile)
- **Read-only root filesystem**: Can be enabled by adding:
  ```yaml
  securityContext:
    readOnlyRootFilesystem: true
  ```
- **Dropped capabilities**: No special capabilities required
- **No privileged escalation**: `allowPrivilegeEscalation: false`

Apply Pod Security Standards to namespace:
```bash
kubectl label namespace diagnostic-operator-system \
  pod-security.kubernetes.io/enforce=restricted \
  pod-security.kubernetes.io/audit=restricted \
  pod-security.kubernetes.io/warn=restricted
```

### Security Context Constraints (SCC)

> **Note**: This section applies to OpenShift deployments only. For vanilla Kubernetes, use Pod Security Standards (PSS) instead.

For OpenShift deployments, diagnostic jobs may need elevated SCC for NFS mounts:

```bash
# Grant privileged SCC to job ServiceAccount
kubectl adm policy add-scc-to-user privileged -z diagnostic-job-sa -n diagnostic-operator-system

# Or use less privileged hostmount-anyuid if sufficient
kubectl adm policy add-scc-to-user hostmount-anyuid -z diagnostic-job-sa -n diagnostic-operator-system
```

**Security trade-off**: NFS mounting typically requires elevated permissions. Consider:
- Using CSI drivers that don't require privileged SCC
- Restricting job pod capabilities with `securityContext.capabilities.drop: [ALL]`
- Implementing Pod Security Policies or OPA/Gatekeeper rules

### Image Security

- **Base image**: Uses Red Hat UBI (Universal Base Image) minimal for security updates
- **Image scanning**: Scan images with tools like Trivy, Clair, or Red Hat Quay
- **Image signing**: Sign images and use admission controllers to verify signatures

```bash
# Scan image with Trivy
trivy image <your-registry>/diagnostic-operator-system/diagnostic-operator:v2.0.4
```

---

## Contributing

We welcome contributions to improve the Kubernetes Event-Driven Diagnostic Operator!

### How to Contribute

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/your-feature-name`
3. **Make your changes**:
   - Add new diagnostic rules
   - Improve error handling
   - Enhance documentation
   - Add tests
4. **Test your changes**: Run locally and verify functionality
5. **Commit your changes**: Use descriptive commit messages
   ```bash
   git commit -m "Add storage provisioning diagnostic rule"
   ```
6. **Push to your fork**: `git push origin feature/your-feature-name`
7. **Open a Pull Request**: Describe your changes and link any related issues

### Code Style Guidelines

- Follow Go standard formatting: `go fmt ./...`
- Add comments for exported functions and types
- Use descriptive variable names
- Keep functions focused and single-purpose
- Add logging for important operations

### Testing Guidelines

- Test regex patterns thoroughly with various event messages
- Verify jobs are created correctly with proper mounts
- Test TTL cleanup behavior
- Check error handling and edge cases

### Reporting Issues

When reporting issues, please include:

1. **Operator version**: Image tag or git commit
2. **Kubernetes version**: `kubectl version`
3. **Symptom description**: What's not working?
4. **Steps to reproduce**: How to trigger the issue
5. **Logs**: Operator logs and job pod logs
   ```bash
   kubectl logs -n diagnostic-operator-system deployment/diagnostic-operator
   ```
6. **Event details**: The Warning event that triggered the issue
   ```bash
   kubectl get event <event-name> -o yaml
   ```

### Roadmap & Future Enhancements

Potential improvements:

- [ ] **Dynamic rule configuration**: CRD-based rule management without rebuilds
- [ ] **Prometheus metrics**: Export metrics for events processed, jobs created, success/failure rates
- [ ] **Alert manager integration**: Send notifications when diagnostics complete or fail
- [ ] **Multi-cluster spoke monitoring**: Direct connection to spoke clusters instead of kubeconfig secrets
- [ ] **Log analysis**: Automated parsing and analysis of must-gather output
- [ ] **Web UI**: Dashboard for viewing diagnostic status and logs
- [ ] **S3 storage backend**: Support for object storage in addition to NFS
- [ ] **Job priority**: Priority queue for critical cluster failures
- [ ] **Retry logic**: Automatic retry for failed diagnostic jobs
- [ ] **Garbage collection tuning**: Configurable TTL per rule

---

## License

This project is licensed under the **Apache License 2.0**.

See the [LICENSE](LICENSE) file for full license text.

```
Copyright 2024-2026 Kubernetes Diagnostic Operator Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

---

## Acknowledgments

- Built with [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) framework
- Inspired by OpenShift's [must-gather](https://github.com/openshift/must-gather) diagnostic tool
- Uses [Red Hat Universal Base Image (UBI)](https://www.redhat.com/en/blog/introducing-red-hat-universal-base-image) for container runtime

---

## Support & Community

- **Issues**: [GitHub Issues](https://github.com/username/event-driven-diagnostic-operator/issues)
- **Discussions**: [GitHub Discussions](https://github.com/username/event-driven-diagnostic-operator/discussions)
- **Email**: pmohanra@redhat.com

---

