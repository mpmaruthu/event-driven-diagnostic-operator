package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

const operatorNamespace = "diagnostic-operator-system"

// copySecretToNamespace reads a secret from the spoke cluster's namespace
// and creates a copy in the operator namespace so the Job pod can mount it.
func (r *EventReconciler) copySecretToNamespace(ctx context.Context, clusterName string) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	srcName := fmt.Sprintf("%s-admin-kubeconfig", clusterName)
	dstName := fmt.Sprintf("diag-kubeconfig-%s", clusterName)

	// Read the source secret from the spoke namespace
	var srcSecret corev1.Secret
	key := types.NamespacedName{Namespace: clusterName, Name: srcName}
	if err := r.Get(ctx, key, &srcSecret); err != nil {
		return "", fmt.Errorf("reading secret %s/%s: %w", clusterName, srcName, err)
	}

	// Check if the copy already exists (idempotent)
	var existing corev1.Secret
	dstKey := types.NamespacedName{Namespace: operatorNamespace, Name: dstName}
	if err := r.Get(ctx, dstKey, &existing); err == nil {
		log.Info("Kubeconfig secret copy already exists", "name", dstName)
		return dstName, nil
	}

	// Create a copy in the operator namespace
	localSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dstName,
			Namespace: operatorNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "diagnostic-operator",
				"diagnostic-operator/cluster":  clusterName,
			},
		},
		Type: srcSecret.Type,
		Data: srcSecret.Data,
	}

	if err := r.Create(ctx, localSecret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return dstName, nil
		}
		return "", fmt.Errorf("creating local secret copy %s: %w", dstName, err)
	}

	log.Info("Copied kubeconfig secret to operator namespace",
		"src", fmt.Sprintf("%s/%s", clusterName, srcName),
		"dst", fmt.Sprintf("%s/%s", operatorNamespace, dstName))
	return dstName, nil
}

// CreateDiagnosticJob spins up a Pod to run the actual analysis
// [Step 6: Run must-gather with specific image]
func (r *EventReconciler) CreateDiagnosticJob(ctx context.Context, clusterName, image string) error {
	log := ctrl.LoggerFrom(ctx)

	if image == "" {
		image = "registry.redhat.io/openshift4/ose-must-gather:latest"
	}

	// Copy the kubeconfig secret from the spoke namespace into the operator
	// namespace so the Job pod can mount it (cross-namespace mounts are not
	// supported by Kubernetes).
	localSecretName, err := r.copySecretToNamespace(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("copying kubeconfig for cluster %s: %w", clusterName, err)
	}

	ttl := int32(3600)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("diag-%s-", clusterName),
			Namespace:    operatorNamespace,
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "diagnostic-job-sa",
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					Volumes: []corev1.Volume{
						{
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: localSecretName,
								},
							},
						},
						{
							Name: "nfs-storage",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "logs-pvc",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "must-gather-executor",
							Image:   image,
							Command: []string{"/usr/bin/oc"},
							Args: []string{
								"adm", "must-gather",
								"--dest-dir=/mnt/nfs/logs/" + clusterName,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "KUBECONFIG",
									Value: "/etc/secret/kubeconfig",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "kubeconfig",
									MountPath: "/etc/secret",
									ReadOnly:  true,
								},
								{
									Name:      "nfs-storage",
									MountPath: "/mnt/nfs",
								},
							},
						},
					},
				},
			},
		},
	}

	log.Info("Spawning diagnostic Job",
		"cluster", clusterName,
		"image", image,
		"kubeconfigSecret", localSecretName)
	return r.Create(ctx, job)
}