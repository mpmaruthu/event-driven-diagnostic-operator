package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	corev1 "k8s.io/api/core/v1"
)

// EventInfo represents simplified event information
type EventInfo struct {
	Namespace  string
	Name       string
	Type       string
	Reason     string
	Message    string
	LastSeen   string
	ObjectKind string
	ObjectName string
}

// QueryWarningEvents executes kubectl command to get Warning events in a namespace
func QueryWarningEvents(namespace string, kubeconfig string) ([]EventInfo, error) {
	args := []string{
		"get", "events",
		"-n", namespace,
		"--field-selector", "type=Warning",
		"-o", "json",
	}

	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}

	cmd := exec.CommandContext(
		context.Background(),
		"kubectl",
		args...,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl command failed: %w, output: %s", err, string(output))
	}

	return parseEventOutput(output)
}

// QueryAllNamespacesWarningEvents queries Warning events across all namespaces
func QueryAllNamespacesWarningEvents(kubeconfig string) (map[string][]EventInfo, error) {
	args := []string{
		"get", "events",
		"--all-namespaces",
		"--field-selector", "type=Warning",
		"-o", "json",
	}

	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}

	cmd := exec.CommandContext(
		context.Background(),
		"kubectl",
		args...,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl command failed: %w, output: %s", err, string(output))
	}

	return parseEventsByNamespace(output)
}

func parseEventOutput(output []byte) ([]EventInfo, error) {
	var eventList corev1.EventList
	if err := json.Unmarshal(output, &eventList); err != nil {
		return nil, fmt.Errorf("failed to parse event output: %w", err)
	}

	events := make([]EventInfo, 0, len(eventList.Items))
	for _, event := range eventList.Items {
		events = append(events, EventInfo{
			Namespace:  event.Namespace,
			Name:       event.Name,
			Type:       event.Type,
			Reason:     event.Reason,
			Message:    event.Message,
			LastSeen:   event.LastTimestamp.String(),
			ObjectKind: event.InvolvedObject.Kind,
			ObjectName: event.InvolvedObject.Name,
		})
	}

	return events, nil
}

func parseEventsByNamespace(output []byte) (map[string][]EventInfo, error) {
	var eventList corev1.EventList
	if err := json.Unmarshal(output, &eventList); err != nil {
		return nil, fmt.Errorf("failed to parse event output: %w", err)
	}

	eventsByNs := make(map[string][]EventInfo)
	for _, event := range eventList.Items {
		info := EventInfo{
			Namespace:  event.Namespace,
			Name:       event.Name,
			Type:       event.Type,
			Reason:     event.Reason,
			Message:    event.Message,
			LastSeen:   event.LastTimestamp.String(),
			ObjectKind: event.InvolvedObject.Kind,
			ObjectName: event.InvolvedObject.Name,
		}
		eventsByNs[event.Namespace] = append(eventsByNs[event.Namespace], info)
	}

	return eventsByNs, nil
}
