//go:build !js

package native

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	redis "github.com/redis/go-redis/v9"
)

// RedisKVStore is a networked KeyValBackend backed by Redis (or a
// wire-compatible server such as Valkey). Values are stored as JSON
// bytes (valueToKVBytes / kvBytesToValue). Opened with a redis:// URL.
type RedisKVStore struct {
	client *redis.Client
}

var _ KeyValBackend = (*RedisKVStore)(nil)

// kvRedisTimeout bounds each individual Redis operation.
const kvRedisTimeout = 10 * time.Second

// NewRedisKVStore connects to Redis using a redis:// connection URL and
// verifies the connection with a PING.
func NewRedisKVStore(dsn string) (*RedisKVStore, error) {
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, fmt.Errorf("redis url: %w", redactError(err, dsn))
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), kvRedisTimeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis connect: %w", redactError(err, dsn))
	}
	return &RedisKVStore{client: client}, nil
}

func (s *RedisKVStore) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), kvRedisTimeout)
}

func (s *RedisKVStore) Get(key string) (Value, bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	b, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return Value{}, false, nil
	}
	if err != nil {
		return Value{}, false, err
	}
	v, err := kvBytesToValue(b)
	if err != nil {
		return Value{}, false, err
	}
	return v, true, nil
}

func (s *RedisKVStore) Set(key string, val Value) error {
	b, err := valueToKVBytes(val)
	if err != nil {
		return err
	}
	ctx, cancel := s.ctx()
	defer cancel()
	return s.client.Set(ctx, key, b, 0).Err()
}

func (s *RedisKVStore) Delete(key string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	n, err := s.client.Del(ctx, key).Result()
	return n > 0, err
}

func (s *RedisKVStore) Has(key string) (bool, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	n, err := s.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (s *RedisKVStore) Keys(prefix string) ([]string, error) {
	ctx, cancel := s.ctx()
	defer cancel()
	match := "*"
	if prefix != "" {
		match = prefix + "*"
	}
	var out []string
	iter := s.client.Scan(ctx, 0, match, 0).Iterator()
	for iter.Next(ctx) {
		out = append(out, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out) // SCAN order is unspecified; sort for determinism
	return out, nil
}

func (s *RedisKVStore) Kind() string { return "redis" }

func (s *RedisKVStore) Close() error { return s.client.Close() }
