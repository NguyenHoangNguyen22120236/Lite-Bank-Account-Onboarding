package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	const key = "LITE_BANK_DOTENV_TEST_VALUE"
	for _, tc := range []struct {
		name     string
		contents string
		existing bool
		want     string
	}{
		{"loads_file", key + "=from_file\n", false, "from_file"},
		{"preserves_environment", key + "=from_file\n", true, "from_environment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Restore any original value while ensuring the load case starts unset.
			t.Setenv(key, "from_environment")
			if !tc.existing {
				if err := os.Unsetenv(key); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			if err := LoadDotEnv(path); err != nil {
				t.Fatal(err)
			}
			if got := os.Getenv(key); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")); err != nil {
		t.Fatalf("missing optional file must be allowed: %v", err)
	}
}

func TestLoadDotEnvMalformedFile(t *testing.T) {
	const sensitive = "synthetic-private-value"
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("LITE_BANK_DOTENV_TEST_VALUE=\""+sensitive+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := LoadDotEnv(path)
	if err == nil {
		t.Fatal("malformed file must return an error")
	}
	if strings.Contains(err.Error(), sensitive) || strings.Contains(err.Error(), path) {
		t.Fatal("error exposes configuration contents or file path")
	}
}
