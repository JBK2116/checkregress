package models

import "fmt"

// Route represents a proxy route in the application.
type Route struct {
	// Name sets the name of the routing group.
	Name string `yaml:"name"`
	// Legacy is the full url of the legacy endpoint.
	Legacy string `yaml:"legacy"`
	// Secondary is the full url of the shadow legacy endpoint.
	Secondary string `yaml:"secondary,omitempty"`
	// Candidate is the full url of the new migration endpoint.
	Candidate string `yaml:"candidate"`
}

// ValidateRouteConfig ensures that a route configuration variable is properly configured.
func (r *Route) ValidateRouteConfig() {
	const missingFieldMessage = "invalid yaml configuration (missing required field)"

	if r.Name == "" {
		panic(fmt.Sprintf("%s: name", missingFieldMessage))
	}
	if r.Legacy == "" {
		panic(fmt.Sprintf("%s: legacy", missingFieldMessage))
	}
	if r.Candidate == "" {
		panic(fmt.Sprintf("%s: candidate", missingFieldMessage))
	}
}

// ApplyDefaults sets appropriate defaults for a route variable in the yaml configuration file.
func (r *Route) ApplyDefaults() {
	if r.Secondary == "" {
		r.Secondary = r.Legacy
	}
}
