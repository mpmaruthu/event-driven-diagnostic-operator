package controller

import (
	"context"
	"fmt"
	
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateDiagnosticJob spins up a Pod to run the actual analysis
// [Step 6: Run must-gather with specific image]
func (r *EventReconciler) CreateDiagnosticJob(ctx context.Context, clusterName, image string) error {
	
	// Default image if none provided
	if image == "" {
		image = "registry.redhat.io/openshift4/ose-must-gather:latest"
	}

	// Define the Job
	// This Job acts as the agent that exports logs to NFS (Step 7)
	ttl := int32(3600) // Auto-delete job after 1 hour (Garbage Collection Step 8)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("diag-%s-", clusterName),
			Namespace:    "diagnostic-operator-system",
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl, // [Step 8: Garbage Collection]
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "diagnostic-job-sa",
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Volumes: []corev1.Volume{
						{
							// Mount the Spoke Cluster's Kubeconfig Secret
							// [Step 5: Export cluster's KUBECONFIG]
							Name: "kubeconfig",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: fmt.Sprintf("%s-admin-kubeconfig", clusterName),
								},
							},
						},
						{
							// [Step 7: NFS Storage Mount]
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
							Name:  "must-gather-executor",
							Image: image, // Data-driven image
							Args: []string{
								"adm", "must-gather",
								"--dest-dir=/mnt/nfs/logs/" + clusterName, // Write directly to NFS
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

	fmt.Printf("Spawning Job for cluster: %s with image: %s\n", clusterName, image)
	return r.Create(ctx, job)
}