package postgres

import (
	"context"
	"strings"
	"testing"
)

func TestOpenRejectsInvalidDSN(t *testing.T) {
	for _, dsn := range []string{"", "   ", "postgres://user:private-password@localhost:invalid/database", "postgres://user:private-password@%zz/database"} {
		t.Run(dsnLabel(dsn), func(t *testing.T) {
			pool, err := Open(context.Background(), dsn)
			if pool != nil {
				pool.Close()
				t.Fatal("invalid DSN returned a pool")
			}
			if err == nil {
				t.Fatal("invalid DSN must return an error")
			}
			if strings.Contains(err.Error(), "private-password") || (strings.TrimSpace(dsn) != "" && strings.Contains(err.Error(), dsn)) {
				t.Fatal("error exposes connection credentials or the DSN")
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
