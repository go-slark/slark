package cache

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dtm-labs/rockscache"
	"github.com/go-slark/slark/pkg/sf"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"time"
)

// db cache

type Cache struct {
	rocks  *rockscache.Client
	sf     *sf.SingleFlight
	err    error // not found error
	expiry time.Duration
}

type Option func(*Cache)

func Error(err error) Option {
	return func(c *Cache) {
		c.err = err
	}
}

func Expiry(expiry time.Duration) Option {
	return func(c *Cache) {
		c.expiry = expiry
	}
}

func New(redis redis.UniversalClient, opts ...Option) *Cache {
	c := &Cache{
		rocks: rockscache.NewClient(redis, rockscache.NewDefaultOptions()),
		err:    gorm.ErrRecordNotFound,
		expiry: time.Hour * 24 * 7,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Cache) Fetch(ctx context.Context, key string, v any, fn func(any) error) (bool, error) {
	var found bool // db from
	data, err := c.rocks.Fetch2(ctx, key, c.expiry, func() (string, error) {
		err := fn(v)
		if err != nil {
			if errors.Is(err, c.err) {
				return "", nil
			}
			return "", err
		}
		found = true
		data, err := json.Marshal(v)
		return string(data), err
	})
	if err != nil {
		return found, err
	}
	if len(data) == 0 {
		return found, c.err
	}
	return found, json.Unmarshal([]byte(data), v)
}

/*
 key: db unique index key
 kf : db primary index key
 fn: query primary index by unique index
 f: query value by primary index
*/

func (c *Cache) FetchIndex(ctx context.Context, key string, kf func(any) string, v any, fn, f func(any) error) error {
	var pk any
	found, err := c.Fetch(ctx, key, &pk, fn)
	if err != nil {
		return err
	}
	if found {
		data, e := json.Marshal(v)
		if e != nil {
			return nil
		}
		_ = c.rocks.RawSet(ctx, kf(pk), string(data), c.expiry)
		return nil
	}
	_, err = c.Fetch(ctx, kf(pk), v, f)
	return err
}

// insert delete update

func (c *Cache) Exec(ctx context.Context, v any, f func(any) error, keys ...string) error {
	err := f(v)
	if err != nil {
		return err
	}
	return c.Deletes(ctx, keys)
}

func (c *Cache) Delete(ctx context.Context, key string) error {
	return c.rocks.TagAsDeleted2(ctx, key)
}

func (c *Cache) Deletes(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.rocks.TagAsDeletedBatch2(ctx, keys)
}
