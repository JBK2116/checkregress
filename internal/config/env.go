package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// EnvConfig stores the environment variable used in the application.
type EnvConfig struct {
	// DatabaseURL stores the full url of the database used in this application.
	DatabaseURL string
}

const (
	envFileName = ".env"
)

// loadEnvFromDir loads the ENV file from the given directory.
func loadEnvFromDir(dir string) EnvConfig {
	return loadEnvFromPath(filepath.Join(dir, envFileName))
}

// loadEnvFromPath loads the ENV file from the given path and handles validation.
func loadEnvFromPath(path string) EnvConfig {
	err := godotenv.Load(path)
	if err != nil {
		panic("error reading env configuration file")
	}
	var conf EnvConfig

	conf.DatabaseURL = os.Getenv("DatabaseURL")
	conf.validateEnvConfig()
	return conf
}

// LoadEnv loads the ENV config file from the current working directory.
func LoadEnv() EnvConfig {
	wd, err := os.Getwd()
	if err != nil {
		panic("error getting env configuration file path")
	}
	return loadEnvFromDir(wd)
}

// validateEnvConfig ensures that the application ".env" file is valid for use.
func (c *EnvConfig) validateEnvConfig() {
	const missingFieldMessage = "invalid env configuration (missing required field)"
	if c.DatabaseURL == "" {
		panic(fmt.Sprintf("%s: DatabaseURL", missingFieldMessage))
	}
}
