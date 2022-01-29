package main

import (
	"bytes"
	"encoding/gob"
	"strings"

	"github.com/coocood/freecache"
)

const (
	defaultCacheSize = 1000 << 20 // 1000 MB cache
	defaultExpiry    = 0          // No expiry
	stringSeparator  = "<>"       // Assuming the separator wont be used in any case.
)

// Cache objects.
type Cache struct {
	fc *freecache.Cache
}

type GeneralCacheItem struct {
	Value interface{}
}

// NewCache will create a new freecache
func NewCache() Cache {
	return Cache{fc: freecache.NewCache(defaultCacheSize)}
}

// Set a string value to cache.
func (c *Cache) SetString(key string, val ...string) error {
	var value = strings.Join(val, stringSeparator)
	return c.setWithExpiry(key, []byte(value), int(defaultExpiry))
}

func (c *Cache) setWithExpiry(key string, val []byte, expiry int) error {
	return c.fc.Set([]byte(key), val, expiry)
}

// Get a string value from cache.
func (c *Cache) GetString(key string) ([]string, error) {
	val, err := c.fc.Get([]byte(key))
	if err != nil {
		return nil, err
	}

	return strings.Split(string(val), stringSeparator), nil
}

// Set a generic cache value.
// These are about 100 micro-seconds (3 times) slower
// than the string cache above. It's slower because of
// encoding and decoding
func (c *Cache) Set(key string, value interface{}) error {
	var valueBytesBuffer bytes.Buffer

	// gob can only encode struct items.
	// It can't encode arrays properly
	cacheItem := GeneralCacheItem{value}

	enc := gob.NewEncoder(&valueBytesBuffer)
	err := enc.Encode(cacheItem)
	if err != nil {
		return err
	}

	return c.fc.Set([]byte(key), valueBytesBuffer.Bytes(), int(defaultExpiry))
}

// Get a value from cache.
func (c *Cache) Get(key string) (interface{}, error) {
	valueBytes, err := c.fc.Get([]byte(key))
	if err != nil {
		return nil, err
	}

	var cacheItem GeneralCacheItem

	dec := gob.NewDecoder(bytes.NewReader(valueBytes))
	err = dec.Decode(&cacheItem)
	if err != nil {
		return nil, err
	}

	return cacheItem.Value, nil
}

// Delete a value from cache.
func (c *Cache) Delete(key string) (bool, error) {
	affected := c.fc.Del([]byte(key))
	return affected, nil
}

// Clear everything in the cache.
func (c *Cache) Clear() {
	c.fc.Clear()
}
