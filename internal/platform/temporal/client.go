package temporal

import (
	"context"
	"errors"
	"go.temporal.io/sdk/client"
	"os"
)

type Config struct{ Address, Namespace, TaskQueue string }

func FromEnv() Config {
	return Config{
		Address:   setting("TEMPORAL_ADDRESS", "localhost:7233"),
		Namespace: setting("TEMPORAL_NAMESPACE", "default"),
		TaskQueue: setting("TEMPORAL_TASK_QUEUE", "bank-onboarding"),
	}
}

func setting(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// Open owns no retry loop. The caller bounds connection setup and closes the client.
func Open(ctx context.Context, cfg Config) (client.Client, error) {
	c, err := client.DialContext(ctx, client.Options{HostPort: cfg.Address, Namespace: cfg.Namespace})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("could not connect to Temporal")
	}
	return c, nil
}
