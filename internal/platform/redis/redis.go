// Package redis wraps the go-redis client for the platform's ephemeral store:
// sessions, magic-link tokens, and rate-limit counters (ADR-0005). Redis is
// treated as lossy — nothing durable lives here.
package redis

import (
	"context"

	"github.com/pubkraal/paddock/internal/platform/config"
	goredis "github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client.
type Client struct {
	rdb *goredis.Client
}

// New wraps an existing go-redis client (used in tests).
func New(rdb *goredis.Client) *Client {
	return &Client{rdb: rdb}
}

// Open constructs a client from config. The connection is lazy; call Ping to
// verify reachability.
func Open(cfg config.Redis) *Client {
	return New(goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}))
}

// Redis exposes the underlying go-redis client for components that need it.
func (c *Client) Redis() *goredis.Client {
	return c.rdb
}

// Ping verifies the server is reachable.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close releases the client.
func (c *Client) Close() error {
	return c.rdb.Close()
}
