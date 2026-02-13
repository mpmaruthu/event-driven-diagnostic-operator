package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseClusterName(t *testing.T) {
	r := &EventReconciler{}

	tests := []struct {
		name     string
		event    *corev1.Event
		expected string
	}{
		{
			name: "Strategy A: ClusterDeployment InvolvedObject",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "ClusterDeployment",
					Name: "spoke-prod-1",
				},
				Message: "installation failed",
			},
			expected: "spoke-prod-1",
		},
		{
			name: "Strategy A: ManagedCluster InvolvedObject",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "open-cluster-management"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "ManagedCluster",
					Name: "spoke-staging-3",
				},
				Message: "cluster unreachable",
			},
			expected: "spoke-staging-3",
		},
		{
			name: "Strategy B: namespace with spoke- prefix",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "spoke-prod-1"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Pod",
					Name: "etcd-member-0",
				},
				Message: "etcd database corruption detected",
			},
			expected: "spoke-prod-1",
		},
		{
			name: "Strategy C: etcd corruption message with cluster keyword",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Pod",
					Name: "etcd-0",
				},
				Message: "etcd database corruption detected on cluster spoke-prod-1",
			},
			expected: "spoke-prod-1",
		},
		{
			name: "Strategy C: ClusterDeployment in message text",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "hive"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Job",
					Name: "install-job-123",
				},
				Message: "ClusterDeployment spoke-prod-1 failed provisioning",
			},
			expected: "spoke-prod-1",
		},
		{
			name: "Strategy C: 'on' keyword without 'cluster'",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "monitoring"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Deployment",
					Name: "prometheus",
				},
				Message: "Network CNI failed on spoke-edge-7",
			},
			expected: "spoke-edge-7",
		},
		{
			name: "no match returns empty string",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Pod",
					Name: "coredns-abc",
				},
				Message: "Back-off restarting failed container",
			},
			expected: "",
		},
		{
			name: "Strategy A takes priority over Strategy C",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "spoke-wrong"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "ClusterDeployment",
					Name: "spoke-correct",
				},
				Message: "ClusterDeployment spoke-from-message failed",
			},
			expected: "spoke-correct",
		},
		{
			name: "Strategy B takes priority over Strategy C",
			event: &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{Namespace: "spoke-from-ns"},
				InvolvedObject: corev1.ObjectReference{
					Kind: "Pod",
					Name: "etcd-0",
				},
				Message: "error on cluster spoke-from-message",
			},
			expected: "spoke-from-ns",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := r.parseClusterName(tc.event)
			if got != tc.expected {
				t.Errorf("parseClusterName() = %q, want %q", got, tc.expected)
			}
		})
	}
}
