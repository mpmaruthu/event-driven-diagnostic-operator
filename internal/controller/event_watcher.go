package controller

import (
	"context"
	"strings"
    
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"diagnostic-operator/internal/config"
)

// EventReconciler watches for specific Warning events
type EventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Rules  []config.DiagnosticRule
}

// SetupWithManager sets up the controller with filtering
// [Step 2: Cluster events receiver (types=Warning)]
func (r *EventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Event{}). // Watch Events
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				// Only process "Warning" events
				evt := e.Object.(*corev1.Event)
				return evt.Type == "Warning"
			},
			UpdateFunc: func(e event.UpdateEvent) bool { return false }, // Ignore updates
			DeleteFunc: func(e event.DeleteEvent) bool { return false }, // Ignore deletes
		}).
		Complete(r)
}

// Reconcile is the main logic loop
func (r *EventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var k8sEvent corev1.Event
	if err := r.Get(ctx, req.NamespacedName, &k8sEvent); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	message := k8sEvent.Message

	// [Step 3: Output Parser]
	// Extract Spoke Cluster Name (e.g., from "ClusterDeployment cluster-1 failed...")
	spokeClusterName := r.parseClusterName(message)
	if spokeClusterName == "" {
		return ctrl.Result{}, nil // Not a cluster-related event
	}

	// [Step 4: Image Name Extractor]
	image := r.determineImage(message)

	// [Step 5, 6, 7]: Offload to the Job Creator
	// We do NOT run must-gather here (blocking). We spawn a K8s Job.
	err := r.CreateDiagnosticJob(ctx, spokeClusterName, image)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *EventReconciler) parseClusterName(msg string) string {
	// Simple string parsing or regex to extract cluster name
	// In reality, this would be robust regex
	if strings.Contains(msg, "ClusterDeployment") {
		parts := strings.Fields(msg)
		if len(parts) > 1 {
			return parts[1] // Mock logic
		}
	}
	return ""
}

func (r *EventReconciler) determineImage(msg string) string {
	for _, rule := range r.Rules {
		if rule.Pattern.MatchString(msg) {
			return rule.Image
		}
	}
	return "" // Default
}