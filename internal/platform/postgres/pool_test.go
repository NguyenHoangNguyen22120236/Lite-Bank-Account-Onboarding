package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenRejectsInvalidDSN(t *testing.T) {
	for _, dsn := range []string{"", "   ", "postgres://user:private-password@localhost:invalid/database", "postgres://user:private-password@%zz/database"} {
		t.Run(dsnLabel(dsn), func(t *testing.T) {
			pool, err := Open(context.Background(), dsn)
			if pool != nil {
				t.Cleanup(pool.Close)
			}
			require.True(t, pool == nil, "invalid DSN returned a pool")
			require.Error(t, err)
			require.False(t, strings.Contains(err.Error(), "private-password"), "error exposes connection credentials")
			if strings.TrimSpace(dsn) != "" {
				require.False(t, strings.Contains(err.Error(), dsn), "error exposes the DSN")
			}
		})
	}
}

func dsnLabel(dsn string) string {
	if strings.TrimSpace(dsn) == "" {
		return "empty"
	}
	if strings.Contains(dsn, "%zz") {
		return "invalid_escape"
	}
	return "invalid_port"
}
