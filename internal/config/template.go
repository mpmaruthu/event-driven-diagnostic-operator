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

const DefaultMustGatherImage = "image-registry.openshift-image-registry.svc:5000/diagnostic-operator-system/ose-must-gather:latest"

// LoadTemplates returns the pre-defined data collection template.
// If defaultImage is empty, DefaultMustGatherImage is used.
func LoadTemplates(defaultImage string) []DiagnosticRule {
	if defaultImage == "" {
		defaultImage = DefaultMustGatherImage
	}
	return []DiagnosticRule{
		{
			Name:    "ETCD Corruption",
			Pattern: regexp.MustCompile(`(?i)etcd.*database.*corruption`),
			Image:   defaultImage,
		},
		{
			Name:    "OVN Network Failure",
			Pattern: regexp.MustCompile(`(?i)Network.*CNI.*failed`),
			Image:   defaultImage,
		},
	}
}