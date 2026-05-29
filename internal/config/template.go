package config

import (
	"regexp"
)

// DiagnosticRule maps an error pattern to a specific image
type DiagnosticRule struct {
	Name    string
	Pattern *regexp.Regexp
	Image   string // The specific must-gather image
}

// LoadTemplates returns the pre-defined data collection template
// [Matches the top box in your diagram]
func LoadTemplates() []DiagnosticRule {
	return []DiagnosticRule{
		{
			Name:    "ETCD Corruption",
			Pattern: regexp.MustCompile(`(?i)etcd.*database.*corruption`),
			Image:   "image-registry.openshift-image-registry.svc:5000/diagnostic-operator-system/ose-must-gather:latest",
		},
		{
			Name:    "OVN Network Failure",
			Pattern: regexp.MustCompile(`(?i)Network.*CNI.*failed`),
			Image:   "image-registry.openshift-image-registry.svc:5000/diagnostic-operator-system/ose-must-gather:latest",
		},
	}
}