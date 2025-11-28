// Package cache provides caching infrastructure for the Knowledge Engine.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss indicates a cache miss.
var ErrCacheMiss = errors.New("cache miss")

// Client defines the cache interface.
type Client interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
	Close() error
}

// RedisClient implements cache using Redis.
type RedisClient struct {
	client *redis.Client
	prefix string
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
	Prefix   string
}

// NewRedisClient creates a new Redis cache client.
func NewRedisClient(cfg RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "ke:"
	}

	return &RedisClient{
		client: client,
		prefix: prefix,
	}, nil
}

// Get retrieves a value from cache.
func (c *RedisClient) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, c.prefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, fmt.Errorf("redis get: %w", err)
	}
	return val, nil
}

// Set stores a value in cache with TTL.
func (c *RedisClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.client.Set(ctx, c.prefix+key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set: %w", err)
	}
	return nil
}

// Delete removes a value from cache.
func (c *RedisClient) Delete(ctx context.Context, key string) error {
	if err := c.client.Del(ctx, c.prefix+key).Err(); err != nil {
		return fmt.Errorf("redis delete: %w", err)
	}
	return nil
}

// DeleteByPrefix removes all keys with the given prefix.
func (c *RedisClient) DeleteByPrefix(ctx context.Context, prefix string) error {
	pattern := c.prefix + prefix + "*"
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return fmt.Errorf("redis delete by prefix: %w", err)
		}
	}
	
	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan: %w", err)
	}
	
	return nil
}

// Close closes the Redis connection.
func (c *RedisClient) Close() error {
	return c.client.Close()
}

// Publish publishes a message to a Redis channel.
func (c *RedisClient) Publish(ctx context.Context, channel string, message interface{}) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	
	if err := c.client.Publish(ctx, c.prefix+channel, data).Err(); err != nil {
		return fmt.Errorf("redis publish: %w", err)
	}
	
	return nil
}

// Subscribe subscribes to a Redis channel.
func (c *RedisClient) Subscribe(ctx context.Context, channel string) (<-chan []byte, func(), error) {
	sub := c.client.Subscribe(ctx, c.prefix+channel)
	
	ch := make(chan []byte, 100)
	done := make(chan struct{})
	
	go func() {
		defer close(ch)
		for {
			select {
			case <-done:
				return
			case msg := <-sub.Channel():
				if msg != nil {
					ch <- []byte(msg.Payload)
				}
			}
		}
	}()
	
	unsubscribe := func() {
		close(done)
		_ = sub.Close()
	}
	
	return ch, unsubscribe, nil
}

// MemoryClient implements an in-memory cache for development.
type MemoryClient struct {
	mu      sync.RWMutex
	data    map[string]cacheEntry
	maxSize int
}

type cacheEntry struct {
	value     []byte
	expiresAt time.Time
}

// NewMemoryClient creates a new in-memory cache client.
func NewMemoryClient(maxSize int) *MemoryClient {
	if maxSize <= 0 {
		maxSize = 10000
	}
	
	c := &MemoryClient{
		data:    make(map[string]cacheEntry),
		maxSize: maxSize,
	}
	
	// Start cleanup goroutine
	go c.cleanup()
	
	return c
}

// Get retrieves a value from cache.
func (c *MemoryClient) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, ok := c.data[key]
	if !ok {
		return nil, ErrCacheMiss
	}
	
	if time.Now().After(entry.expiresAt) {
		return nil, ErrCacheMiss
	}
	
	return entry.value, nil
}

// Set stores a value in cache with TTL.
func (c *MemoryClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Simple eviction if at max size
	if len(c.data) >= c.maxSize {
		c.evictOldest()
	}
	
	c.data[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	
	return nil
}

// Delete removes a value from cache.
func (c *MemoryClient) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	delete(c.data, key)
	return nil
}

// DeleteByPrefix removes all keys with the given prefix.
func (c *MemoryClient) DeleteByPrefix(ctx context.Context, prefix string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for key := range c.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.data, key)
		}
	}
	
	return nil
}

// Close is a no-op for memory cache.
func (c *MemoryClient) Close() error {
	return nil
}

// evictOldest removes the entry with the earliest expiration.
func (c *MemoryClient) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	
	for key, entry := range c.data {
		if oldestKey == "" || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
		}
	}
	
	if oldestKey != "" {
		delete(c.data, oldestKey)
	}
}

// cleanup periodically removes expired entries.
func (c *MemoryClient) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.data {
			if now.After(entry.expiresAt) {
				delete(c.data, key)
			}
		}
		c.mu.Unlock()
	}
}

// CacheKey generates a cache key from components.
func CacheKey(parts ...string) string {
	key := ""
	for i, part := range parts {
		if i > 0 {
			key += ":"
		}
		key += part
	}
	return key
}

// TenantCacheKey generates a tenant-scoped cache key.
func TenantCacheKey(tenantID string, parts ...string) string {
	return CacheKey(append([]string{"t", tenantID}, parts...)...)
}

// ProductCacheKey generates a product-scoped cache key.
func ProductCacheKey(tenantID, productID string, parts ...string) string {
	return CacheKey(append([]string{"t", tenantID, "p", productID}, parts...)...)
}

