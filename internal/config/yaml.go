package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"github.com/JBK2116/checkregress/internal/models"
	"go.yaml.in/yaml/v4"
)

const (
	// YamlFileName is the name of the application yaml configuration file.
	yamlFileName = "config.yml"
	// defaultMaxBodyBytes is the default value for the max_body_bytes field (1 MiB).
	defaultMaxBodyBytes = 1 << 20
	// maxBodyBytesLimit is the upperbound limit for the max_body_bytes field (10 MiB).
	maxBodyBytesLimit = 10 << 20
	// defaultShadowTimeoutMS is the default value for the shadow_timeout_ms field (5 seconds).
	defaultShadowTimeoutMS = 5000
	// maxShadowTimeoutMS is the upperbound limit for the shadow_timeout_ms field (30 seconds).
	maxShadowTimeoutMS = 30000
	// minPort is the lowest valid port number (port 0 is reserved).
	minPort = 1
	// maxPort is the highest valid port number.
	maxPort = 65535
)

// loadYamlFromDir loads the config file named by yamlFileName from the given
// directory.
func loadYamlFromDir(dir string) YamlConfig {
	return loadYamlFromPath(filepath.Join(dir, yamlFileName))
}

// loadYamlFromPath reads, validates and applies defaults for the config file
// at the given path.
func loadYamlFromPath(path string) YamlConfig {
	f, rErr := os.ReadFile(path)
	if rErr != nil {
		panic("error reading yaml configuration file")
	}

	var raw RawYamlConfig

	if uErr := yaml.Unmarshal(f, &raw); uErr != nil {
		panic("error loading yaml configuration file into struct")
	}

	conf := raw.validate()
	return conf
}

// RawYamlConfig represents the raw yaml config parsed from "config.yml".
type RawYamlConfig struct {
	// Listen sets the port used by the reverse proxy.
	Listen string `yaml:"listen"`
	// AdminListen sets the port used to access the adminAPI.
	AdminListen string `yaml:"admin_listen"`
	// MaxBodyBytes sets the max amount of payload data read into memory for a diff.
	MaxBodyBytes int `yaml:"max_body_bytes"`
	// ShadowTimeoutMS sets the max amount of milliseconds a worker has to shadow a proxy request before timing out.
	ShadowTimeoutMS int `yaml:"shadow_timeout_ms"`
	// Routes stores the collection of endpoints to handle in the application.
	Routes []models.RawRoute `yaml:"routes"`
}

// YamlConfig stores the configuration settings in the root "config.yml".
type YamlConfig struct {
	// Listen sets the port used by the reverse proxy.
	Listen string
	// AdminListen sets the port used to access the adminAPI.
	AdminListen string
	// MaxBodyBytes sets the max amount of payload data read into memory for a diff.
	MaxBodyBytes int
	// ShadowTimeoutMS sets the max amount of milliseconds a worker has to shadow a proxy request before timing out.
	ShadowTimeoutMS int
	// Routes stores the collection of endpoints to handle in the application.
	Routes []models.Route
}

// LoadYaml loads the YAML config file (named "config.yml") from the current
// working directory.
func LoadYaml() YamlConfig {
	wd, err := os.Getwd()
	if err != nil {
		panic("error getting yaml configuration file path")
	}
	return loadYamlFromDir(wd)
}

// validate returns a validated YamlConfig.
func (c *RawYamlConfig) validate() YamlConfig {
	const missingFieldMessage = "invalid yaml configuration (missing required field)"
	const invalidFieldMessage = "invalid yaml configuration (field is improperly configured)"
	const duplicateFieldMessage = "invalid yaml configuration (field value already exists)"
	const listenMismatchMessage = "invalid yaml configuration (listen cannot equal admin_listen)"

	if c.Listen == "" {
		panic(fmt.Sprintf("%s: listen", missingFieldMessage))
	}
	if c.AdminListen == "" {
		panic(fmt.Sprintf("%s: admin_listen", missingFieldMessage))
	}
	validateListen(c.Listen)
	validateAdminListen(c.AdminListen)
	if c.Listen == c.AdminListen {
		panic(fmt.Sprintf("%s: (listen = %s) (admin_listen = %s)", listenMismatchMessage, c.Listen, c.AdminListen))
	}
	if c.MaxBodyBytes > maxBodyBytesLimit {
		panic(fmt.Sprintf("%s: max_body_bytes must be less than %d bytes", invalidFieldMessage, maxBodyBytesLimit))
	}
	if c.MaxBodyBytes < 0 {
		panic(fmt.Sprintf("%s: max_body_bytes must be greater than 0 bytes", invalidFieldMessage))
	}
	if c.ShadowTimeoutMS > maxShadowTimeoutMS {
		panic(fmt.Sprintf(
			"%s: shadow_timeout_ms must be less than %d milliseconds",
			invalidFieldMessage, maxShadowTimeoutMS,
		))
	}
	if c.ShadowTimeoutMS < 0 {
		panic(fmt.Sprintf("%s: shadow_timeout_ms must be greater than 0 milliseconds", invalidFieldMessage))
	}
	if len(c.Routes) == 0 {
		panic(fmt.Sprintf("%s: routes", missingFieldMessage))
	}
	c.applyDefaults()

	var conf YamlConfig
	conf.Listen = c.Listen
	conf.AdminListen = c.AdminListen
	conf.MaxBodyBytes = c.MaxBodyBytes
	conf.ShadowTimeoutMS = c.ShadowTimeoutMS

	seenName := map[string]bool{}
	seenLegacy := map[string]bool{}

	routes := make([]models.Route, len(c.Routes))

	for i := range c.Routes {
		// ensure that each route has a unique service name to prevent router panics
		name := c.Routes[i].Name
		if seenName[name] {
			panic(fmt.Sprintf("%s: Name (%s)", duplicateFieldMessage, name))
		}
		seenName[name] = true
		// ensure that each route has a unique legacy url to prevent router configuration mismanagement
		legacy := c.Routes[i].Legacy
		if seenLegacy[legacy] {
			panic(fmt.Sprintf("%s: Legacy (%s)", duplicateFieldMessage, legacy))
		}
		seenLegacy[legacy] = true
		// add this to the yaml config for use
		routes[i] = c.Routes[i].Validate()
	}

	conf.Routes = routes
	return conf
}

// applyDefaults sets appropriate defaults in the yaml configuration file.
func (c *RawYamlConfig) applyDefaults() {
	if c.MaxBodyBytes == 0 {
		c.MaxBodyBytes = defaultMaxBodyBytes
	}
	if c.ShadowTimeoutMS == 0 {
		c.ShadowTimeoutMS = defaultShadowTimeoutMS
	}
}

// validateListen ensures addr is a valid listen address in host:port form with
// a valid port. The host may be empty to bind all interfaces.
func validateListen(addr string) {
	const invalidFieldMessage = "invalid yaml configuration (field is improperly configured)"

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		panic(fmt.Sprintf("%s: listen must be in host:port form", invalidFieldMessage))
	}
	if !validPort(port) {
		panic(fmt.Sprintf("%s: listen must contain a valid port", invalidFieldMessage))
	}
}

// validateAdminListen ensures addr is a valid IP:port address. Unlike listen,
// the host must be a literal IP address rather than a hostname.
func validateAdminListen(addr string) {
	const invalidFieldMessage = "invalid yaml configuration (field is improperly configured)"

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		panic(fmt.Sprintf("%s: admin_listen must be in host:port form", invalidFieldMessage))
	}
	if !validPort(port) {
		panic(fmt.Sprintf("%s: admin_listen must contain a valid port", invalidFieldMessage))
	}
	if net.ParseIP(host) == nil {
		panic(fmt.Sprintf("%s: admin_listen must contain a valid IP address", invalidFieldMessage))
	}
}

// validPort reports whether port is a valid numeric port number.
func validPort(port string) bool {
	p, err := strconv.Atoi(port)
	return err == nil && p >= minPort && p <= maxPort
}
