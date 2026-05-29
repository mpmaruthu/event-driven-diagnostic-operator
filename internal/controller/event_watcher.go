package controller

import (
	"context"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"diagnostic-operator/internal/config"
)

var clusterNamePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:ClusterDeployment|ManagedCluster|cluster)\s+(\S+)`),
	regexp.MustCompile(`(?i)on\s+(?:cluster\s+)?(\S+)`),
}

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

	// [Step 3: Output Parser]
	spokeClusterName := r.parseClusterName(&k8sEvent)
	if spokeClusterName == "" {
		return ctrl.Result{}, nil // Not a cluster-related event
	}

	// [Step 4: Image Name Extractor]
	image := r.determineImage(k8sEvent.Message)

	// [Step 5, 6, 7]: Offload to the Job Creator
	// We do NOT run must-gather here (blocking). We spawn a K8s Job.
	err := r.CreateDiagnosticJob(ctx, spokeClusterName, image)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// parseClusterName extracts the spoke cluster name using multiple strategies:
//   - InvolvedObject metadata (most reliable)
//   - Namespace heuristic (spoke-* naming convention)
//   - Regex patterns on the event message (broadest coverage)
func (r *EventReconciler) parseClusterName(evt *corev1.Event) string {
	// Strategy A: InvolvedObject carries the cluster identity directly.
	kind := evt.InvolvedObject.Kind
	if kind == "ClusterDeployment" || kind == "ManagedCluster" {
		if name := evt.InvolvedObject.Name; name != "" {
			return name
		}
	}

	// Strategy B: Namespace often mirrors the spoke cluster name.
	if ns := evt.Namespace; strings.HasPrefix(ns, "spoke-") {
		return ns
	}

	// Strategy C: Regex extraction from the free-text message.
	for _, re := range clusterNamePatterns {
		if matches := re.FindStringSubmatch(evt.Message); len(matches) > 1 {
			return matches[1]
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