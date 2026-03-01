package cache

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const keyPrefix = "flickmind:"

// Cache wraps Redis with an in-memory fallback if Redis is unavailable.
type Cache struct {
	rdb *redis.Client

	// in-memory fallback
	mu      sync.RWMutex
	entries map[string]memEntry
	usemem  bool
}

type memEntry struct {
	data      []byte
	expiresAt time.Time
}

// New creates a Redis-backed cache. If redisURL is empty, falls back to in-memory.
func New(redisURL string) *Cache {
	c := &Cache{
		entries: make(map[string]memEntry),
	}

	if redisURL == "" {
		log.Println("[Cache] No REDIS_URL set, using in-memory cache")
		c.usemem = true
		go c.cleanup()
		return c
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("[Cache] Invalid REDIS_URL: %v, using in-memory cache", err)
		c.usemem = true
		go c.cleanup()
		return c
	}

	opts.MaxRetries = 3
	opts.DialTimeout = 5 * time.Second
	opts.ReadTimeout = 3 * time.Second
	opts.WriteTimeout = 3 * time.Second

	c.rdb = redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[Cache] Redis unreachable: %v, using in-memory cache", err)
		c.rdb.Close()
		c.rdb = nil
		c.usemem = true
		go c.cleanup()
		return c
	}

	log.Println("[Cache] Connected to Redis")
	return c
}

// Get retrieves a cached value. The dest must be a pointer for JSON unmarshaling.
func (c *Cache) Get(key string, dest interface{}) bool {
	if !c.usemem {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		data, err := c.rdb.Get(ctx, keyPrefix+key).Bytes()
		if err != nil {
			return false
		}
		if err := json.Unmarshal(data, dest); err != nil {
			log.Printf("[Cache] Unmarshal error for %s: %v", key, err)
			return false
		}
		return true
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return false
	}
	if err := json.Unmarshal(e.data, dest); err != nil {
		return false
	}
	return true
}

// Set stores a value with a TTL.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	data, err := json.Marshal(value)
	if err != nil {
		log.Printf("[Cache] Marshal error for %s: %v", key, err)
		return
	}

	if !c.usemem {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := c.rdb.Set(ctx, keyPrefix+key, data, ttl).Err(); err != nil {
			log.Printf("[Cache] Redis SET error for %s: %v", key, err)
		}
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = memEntry{data: data, expiresAt: time.Now().Add(ttl)}
}

// InvalidatePrefix removes all keys with the given prefix.
func (c *Cache) InvalidatePrefix(prefix string) {
	if !c.usemem {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		pattern := keyPrefix + prefix + "*"
		var cursor uint64
		for {
			keys, next, err := c.rdb.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				log.Printf("[Cache] Redis SCAN error: %v", err)
				break
			}
			if len(keys) > 0 {
				c.rdb.Del(ctx, keys...)
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if strings.HasPrefix(k, prefix) {
			delete(c.entries, k)
		}
	}
}

// Close cleanly shuts down the Redis connection.
func (c *Cache) Close() error {
	if c.rdb != nil {
		return c.rdb.Close()
	}
	return nil
}

func (c *Cache) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}
