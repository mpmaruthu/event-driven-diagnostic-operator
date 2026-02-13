package controller

import (
	"encoding/json"
	"net/http"

	"diagnostic-operator/internal/utils"
)

// QueryHandler handles HTTP requests for manual event queries
type QueryHandler struct {
	KubeconfigPath string
}

// HandleEventQuery provides HTTP endpoint for manual event queries
// Endpoint: /query-events?namespace=<namespace>
func (h *QueryHandler) HandleEventQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespace := r.URL.Query().Get("namespace")

	if namespace == "" {
		http.Error(w, "namespace parameter required", http.StatusBadRequest)
		return
	}

	events, err := utils.QueryWarningEvents(namespace, h.KubeconfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"namespace": namespace,
		"count":     len(events),
		"events":    events,
	})
}

// HandleAllNamespacesQuery provides HTTP endpoint for querying all namespaces
// Endpoint: /query-events-all
func (h *QueryHandler) HandleAllNamespacesQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	events, err := utils.QueryAllNamespacesWarningEvents(h.KubeconfigPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalCount := 0
	for _, evts := range events {
		totalCount += len(evts)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total_namespaces": len(events),
		"total_events":     totalCount,
		"events_by_namespace": events,
	})
}
