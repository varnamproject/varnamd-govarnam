package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/coocood/freecache"
)

const (
	defaultCacheSize = 100 << 20      // 100 MB cache
	defaultExpiry    = time.Hour * 24 // Cache for 24 hours
	wordSeparator    = "<>"           // Assuming the separator wont be used in any case.
)

// Cache objects.
type Cache interface {
	Set(lang, word string, val ...string) error
	Get(lang, word string) ([]string, error)
	Delete(lang, word string) (bool, error)
	Clear()
}

// MemCache impliments Cache interface.
type MemCache struct {
	fc *freecache.Cache
}

// NewMemCache will create object of new Cache impliemtation.
func NewMemCache() *MemCache {
	return newCacheWithSize(defaultCacheSize)
}

func newCacheWithSize(size int) *MemCache {
	return &MemCache{fc: freecache.NewCache(size)}
}

// Set lang-word as val to cache.
func (c *MemCache) Set(lang, word string, val ...string) error {
	var value = strings.Join(val, wordSeparator)
	return c.setWithExpiry(lang, word, []byte(value), int(defaultExpiry))
}

func (c *MemCache) setWithExpiry(lang, word string, val []byte, expiry int) error {
	var key = fmt.Sprintf("%s-%s", lang, word)
	return c.fc.Set([]byte(key), val, expiry)
}

// Get lang-word from cache.
func (c *MemCache) Get(lang, word string) ([]string, error) {
	var key = fmt.Sprintf("%s-%s", lang, word)

	val, err := c.fc.Get([]byte(key))
	if err != nil {
		return nil, err
	}

	return strings.Split(string(val), wordSeparator), nil
}

// Delete lang-word from cache.
func (c *MemCache) Delete(lang, word string) (bool, error) {
	var key = fmt.Sprintf("%s-%s", lang, word)
	affected := c.fc.Del([]byte(key))

	return affected, nil
}

// Clear everything in the cache.
func (c *MemCache) Clear() {
	c.fc.Clear()
}
