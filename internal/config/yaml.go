package config

import (
	"fmt"
	"os"
	"path/filepath"

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

	var conf YamlConfig

	if uErr := yaml.Unmarshal(f, &conf); uErr != nil {
		panic("error loading yaml configuration file into struct")
	}
	conf.validateYamlConfig()
	conf.applyDefaults()
	return conf
}

// YamlConfig stores the configuration settings in the root “config.yml“.
type YamlConfig struct {
	// Listen sets the port used by the reverse proxy.
	Listen string `yaml:"listen"`
	// AdminListen sets the port used to access the adminAPI.
	AdminListen string `yaml:"admin_listen"`
	// MaxBodyBytes sets the max amount of payload data read into memory for a diff.
	MaxBodyBytes int `yaml:"max_body_bytes"`
	// ShadowTimeout sets the max amount of seconds a worker has to shadow a proxy request before timing out.
	ShadowTimeoutMS int `yaml:"shadow_timeout_ms"`
	// Routes stores the collection of endpoints to handle in the application.
	Routes []models.Route `yaml:"routes"`
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

// validateYamlConfig ensures that the yaml configuration file is properly configured.
func (c *YamlConfig) validateYamlConfig() {
	const missingFieldMessage = "invalid yaml configuration (missing required field)"
	const invalidFieldMessage = "invalid yaml configuration (field is improperly configured)"

	if c.Listen == "" {
		panic(fmt.Sprintf("%s: listen", missingFieldMessage))
	}
	if c.AdminListen == "" {
		panic(fmt.Sprintf("%s: admin_listen", missingFieldMessage))
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
	for i := range c.Routes {
		c.Routes[i].ValidateRouteConfig()
		c.Routes[i].ApplyDefaults()
	}
}

// applyDefaults sets appropriate defaults in the yaml configuration file.
func (c *YamlConfig) applyDefaults() {
	if c.MaxBodyBytes == 0 {
		c.MaxBodyBytes = defaultMaxBodyBytes
	}
	if c.ShadowTimeoutMS == 0 {
		c.ShadowTimeoutMS = defaultShadowTimeoutMS
	}
}
