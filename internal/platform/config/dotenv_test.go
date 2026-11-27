package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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
				require.NoError(t, os.Unsetenv(key))
			}
			path := filepath.Join(t.TempDir(), ".env")
			require.NoError(t, os.WriteFile(path, []byte(tc.contents), 0600))
			require.NoError(t, LoadDotEnv(path))
			require.Equal(t, tc.want, os.Getenv(key))
		})
	}
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	require.NoError(t, LoadDotEnv(filepath.Join(t.TempDir(), "missing.env")))
}

func TestLoadDotEnvMalformedFile(t *testing.T) {
	const sensitive = "synthetic-private-value"
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte("LITE_BANK_DOTENV_TEST_VALUE=\""+sensitive+"\n"), 0600))
	err := LoadDotEnv(path)
	require.Error(t, err)
	require.NotContains(t, err.Error(), sensitive)
	require.NotContains(t, err.Error(), path)
}
