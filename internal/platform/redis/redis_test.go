package redis_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/pubkraal/paddock/internal/platform/config"
	"github.com/pubkraal/paddock/internal/platform/redis"
)

func TestOpenAndPing(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)

	client := redis.Open(config.Redis{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestPing_Unreachable(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close() // server gone — ping must fail

	client := redis.Open(config.Redis{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Ping(context.Background()); err == nil {
		t.Fatal("expected ping error against a closed server, got nil")
	}
}

func TestRedisAccessor(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)

	client := redis.Open(config.Redis{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	if client.Redis() == nil {
		t.Error("Redis() returned nil")
	}
}
