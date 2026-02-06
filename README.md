# Kubernetes Automated Diagnostic Operator
An event-driven Kubernetes Operator that automatically detects cluster failures, launches the appropriate diagnostic tools (e.g., must-gather), and offloads logs to external storage for root cause or fault slip through analysis.

# 🚀 Features
Event Watcher: Real-time monitoring of Hub Cluster events for Type=Warning.

Smart Routing: Regex-based template engine (internal/config) maps specific error patterns to specific diagnostic images.

Job Offloading: Spawns lightweight Kubernetes Jobs to run diagnostics, ensuring the Operator itself remains non-blocking.

Automated Storage: Mounts RWX (ReadWriteMany) storage to collect and persist logs outside the ephemeral pods.

Self-Cleaning: Uses Kubernetes TTL to automatically garbage collect diagnostic jobs after completion.

# 📋 Prerequisites
Kubernetes Cluster (v1.19+) or OpenShift (v4.x) or OpenShift (v5.x)

Helm (v3+)

Shared Storage (RWX): An NFS or CephFS StorageClass is required to allow multiple diagnostic jobs to write logs simultaneously.

Container Registry: To push the operator image.

# 🛠 Installation
1. Build and Push Image
If you are deploying from source, build the container image first:

docker build -t quay.io/your-org/diagnostic-operator:v1.0.0 .

docker push quay.io/your-org/diagnostic-operator:v1.0.0

2. Deploy via Helm
Install the chart located in ./diagnostic-operator. 

Note: You must specify a valid StorageClass for your cluster (e.g., managed-nfs-storage, ocs-storagecluster-cephfs, or standard).

helm install diag-op ./diagnostic-operator \
  --namespace diagnostic-system \
  --create-namespace \
  --set image.repository=quay.io/your-org/diagnostic-operator \
  --set image.tag=v1.0.0 \
  --set storage.className="managed-nfs-storage" \
  --set storage.size="20Gi"

3. Verify Deployment

kubectl get pods -n diagnostic-system

# ⚙️ Configuration

# 🏗 Architecture & Logic

Detection Flow

Watcher: The Operator listens to the Kubernetes Event API.

Filter: It ignores everything except Type: Warning.

Parser: It extracts the target spoke cluster name (e.g., from ClusterDeployment objects).

Matcher: It checks the error message against internal/config/template.go.

Diagnostic Rules (template.go)

To add new rules, edit internal/config/template.go and rebuild the image:

{
    Name:    "ETCD Failure",
    Pattern: regexp.MustCompile(`(?i)etcd.*database.*corruption`),
    Image:   "quay.io/openshift/etcd-must-gather:latest",
},

# 🧪 Development & Testing

Running Locally

To run the operator code locally (outside the cluster) while connecting to your current kubecontext:

$ make run

$ go run cmd/main.go

Chaos Testing

We include a Chaos Script to simulate failures and verify the pipeline works without breaking real clusters.

1. Make sure the Operator is running.

2. Run the script:

$ ./chaos-test.sh

3. What happens:

Creates a fake Secret (mock kubeconfig).

Injects a fake Event (Etcd corruption warning).

Verifies the Operator launches a Job.

# 🔍 Troubleshooting

Issue #1: PVC remains in Pending state.

Cause: The storage.className in Helm does not match your cluster's storage provisioner.

Fix: Run kubectl get sc to find valid classes and update your Helm command.

Issue #2: Job fails with "Permission Denied" on mount.

Cause: The diagnostic-job-sa ServiceAccount might need SCC (Security Context Constraints) permissions for hostmount or privileged depending on your NFS setup.

Fix: Edit templates/rbac.yaml to include the specific SCC resource name used by your distro.
