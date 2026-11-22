package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

// LoadDotEnv loads optional local configuration without replacing environment variables.
func LoadDotEnv(path string) error {
	err := godotenv.Load(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	// Parsing errors can contain configuration values.
	return errors.New("could not load .env: check file permissions and syntax")
}
