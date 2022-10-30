// cacher provides disk caching of json retrieved from APIs
package cacher

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/crooks/satinv/cacher/satapi"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	cacheExpiryFile string = "expire.json"
	iso8601         string = "2006-01-02T15:04:05Z"
)

var (
	errAPIInit = errors.New("API is not initialised")
)

type Item struct {
	url    bool   // If it's not a URL, it's a file
	expiry int64  // Epoch expiry time
	file   string // Filename associated with the cached content
}

type Cache struct {
	api          *satapi.AuthClient
	apiInit      bool // Test if the API has been initialised
	cacheDir     string
	content      map[string]Item   // A cache of Item structs
	cacheFiles   map[string]string // k=url, v=cacheFile
	cachePeriod  int64             // Seconds
	cacheRefresh bool              // Ignore the cache and grab new URLs
	writeExpiry  bool              // Write expiry data to disk
}

// NewCacher creates and returns a new instance of Cache.  It takes a
// directory name where cache files will be stored and will attempt to create
// that directory if it doesn't exist.
func NewCacher(cacheDir string) *Cache {
	c := new(Cache)
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		err := os.Mkdir(cacheDir, 0755)
		if err != nil {
			log.Fatalf("Cannot create Cache dir: %s", cacheDir)
			panic(err)
		}
		log.Printf("Created cache dir: %s", cacheDir)
	}
	c.cacheDir = cacheDir
	log.Printf("Cache dir set to: %s", c.cacheDir)
	c.cacheFiles = make(map[string]string)
	c.content = make(map[string]Item)
	// This is the only time the expire JSON is read from file.  After this, it resides in memory and only gets written
	// to file.  If the read fails, the Cache is assumed to be empty.
	c.importExpiry()
	return c
}

// GetFilename returns the filename for a given cache item
func (c *Cache) GetFilename(itemKey string) (filename string) {
	var ok bool
	if filename, ok = c.cacheFiles[itemKey]; !ok {
		log.Fatalf("No cache file associated with item \"%s\"", itemKey)
	}
	return
}

// InitAPI constructs a new instance of the Satellite API
func (c *Cache) InitAPI(username, password, cert string) {
	c.api = satapi.NewBasicAuthClient(username, password, cert)
	c.apiInit = true
}

// SetRefresh instructs GetURL to ignore cached files and fetch (and cache) new copies.
func (c *Cache) SetRefresh() {
	c.cacheRefresh = true
	log.Print("Forcing cache refresh")
}

// HasExpired takes a cache item and determines if it needs refreshing
func (c *Cache) HasExpired(itemKey string) (refresh bool, err error) {
	// Test if the cache content map contains this item
	item, ok := c.content[itemKey]
	if !ok {
		err = fmt.Errorf("no cache content associated with %s", itemKey)
		return
	}
	if c.cacheRefresh {
		// Instructed to force a refresh
		log.Printf("Forced refresh of %s", itemKey)
		refresh = true
	} else if _, existErr := os.Stat(item.file); os.IsNotExist(existErr) {
		// File associated with the URL doesn't exist
		log.Printf("Cache file %s for URL %s does not exist", item.file, itemKey)
		refresh = true
	} else if time.Now().Unix() > item.expiry {
		// The Cache entry has expired
		log.Printf("Cache for %s has expired", itemKey)
		refresh = true
	} else {
		refresh = false
	}
	return
}

func (c *Cache) SetExpire(itemKey string, expireEpoch int64) {
	item, ok := c.content[itemKey]
	if !ok {
		log.Printf("Trying to set expire on unknown cache content: %s", itemKey)
	}
	item.expiry = expireEpoch
	c.content[itemKey] = item
}

// AddURL registers a URL with a filename to contain its cached data.  If the URL has no expiry associated with it, a
// new entry is created in the expiry cache and immediately set to expired.
func (c *Cache) AddURL(itemKey, fileName string) {
	item := c.content[itemKey]
	item.url = true
	item.file = path.Join(c.cacheDir, fileName)
	item.expiry = 0
	c.content[itemKey] = item
}

// importExpiry reads the Expiry Cache File and populates the cacheExpiry map.  Entries over 7 days old are ignored.
func (c *Cache) importExpiry() {
	expiryFilePath := path.Join(c.cacheDir, cacheExpiryFile)
	j, err := c.jsonFromFile(expiryFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("%s: Cache file does not exist.  Treating as empty cache", expiryFilePath)
		} else {
			log.Fatalf("%s: Failed to read Cache file: %v", expiryFilePath, err)
		}
		return
	}
	// Populate the cacheExpiry map
	// ageLimit is used to prune out old entries from the Cache File.
	// The hard limit it set to 7 days.
	ageLimit := time.Now().Unix() - (7 * 24 * 60 * 60)
	for k, v := range j.Get("urls").Map() {
		epochExpiry := v.Int()
		if epochExpiry > ageLimit {
			log.Printf("Importing Cache entry: url=%s, expiry=%s", k, timeEpoch(epochExpiry))
			c.SetExpire(k, epochExpiry)
		} else if epochExpiry > 0 {
			// Only log housekeeping for expiry times greater than 0
			log.Printf("Housekeeping old Cache entry: url=%s, expiry=%s", k, timeEpoch(epochExpiry))
		}
	}
}

// exportExpiry writes the cache expiry map to a file in JSON format.
func (c *Cache) exportExpiry() error {
	sj, err := sjson.Set("", "write_time", timestamp())
	if err != nil {
		return err
	}
	// expireMap creates a simple map of item->expireTime (epoch)
	expireMap := make(map[string]int64)
	for k, v := range c.content {
		expireMap[k] = v.expiry
	}
	sj, err = sjson.Set(sj, "urls", expireMap)
	if err != nil {
		return err
	}
	// Add a LF to the end of the file
	sj += "\n"
	fileName := path.Join(c.cacheDir, cacheExpiryFile)
	err = os.WriteFile(fileName, []byte(sj), 0644)
	if err != nil {
		return err
	}
	log.Printf("Expiry cache written to: %s", fileName)
	return nil
}

// UpdateExpiry revises the expiry time of a given cache item.
func (c *Cache) UpdateExpiry(itemKey string, period int64) (newExpire int64) {
	newExpire = expireTime(period)
	c.SetExpire(itemKey, newExpire)
	c.writeExpiry = true
	return
}

// WriteExpiryFile writes the cache inventory to disk if writeExpiry is true.  The writeExpiry boolean indicates that
// at least one cache item has been refreshed.
func (c *Cache) WriteExpiryFile() {
	if c.writeExpiry {
		c.exportExpiry()
		c.writeExpiry = false
	}
}

// getURLFromAPI is called when a cache item has expired and a new copy needs to be grabbed from the API.
func (c *Cache) getURLFromAPI(item string) (gj gjson.Result, err error) {
	if !c.apiInit {
		err = errAPIInit
		return
	}
	log.Printf("Requested retreival of: %s", item)
	bytes, err := c.api.GetJSON(item)
	if err != nil {
		err = fmt.Errorf("unable to parse %s: %v", item, err)
		return
	}
	gj = gjson.ParseBytes(bytes)
	err = c.jsonToFile(c.cacheFiles[item], gj)
	if err != nil {
		err = fmt.Errorf("unable to read JSON: %v", err)
		return
	}
	// We have successfully retreived a URL so update its cache expiry time.
	c.UpdateExpiry(item, c.cachePeriod)
	return
}

// GetURL returns the file content associated with a cache key.  If the cache has expired, the content will instead be
// grabbed from the API.
func (c *Cache) GetURL(item string) (gj gjson.Result, err error) {
	refresh, err := c.HasExpired(item)
	if err != nil {
		return
	}
	if refresh {
		gj, err = c.getURLFromAPI(item)
		return
	}
	// Try and get the requested json from the Cache File
	gj, err = c.jsonFromFile(c.cacheFiles[item])
	if err != nil {
		// Failed to read the Cache File, get it from the API instead
		gj, err = c.getURLFromAPI(item)
	}
	return
}

// GetFile reads a cache item's file from disk and returns it as a byte slice.
func (c *Cache) GetFile(item string) []byte {
	b, err := os.ReadFile(c.cacheFiles[item])
	if err != nil {
		log.Fatal(err)
	}
	return b
}

// SetCacheDuration defines the expiry period in seconds for each cached file.
func (c *Cache) SetCacheDuration(sec int64) {
	c.cachePeriod = sec
	log.Printf("Default cache period set to %d seconds", c.cachePeriod)
}

// expireTime calculates the Epoch time for a Cache expiry
func expireTime(cacheDuration int64) int64 {
	expire := time.Now().Unix() + cacheDuration
	return expire
}

// jsonFromFile takes the filename for a file containing json formatted content
// and returns a gjson Result of the file content.
func (c *Cache) jsonFromFile(filename string) (gjson.Result, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return gjson.Result{}, err
	}
	return gjson.ParseBytes(b), nil
}

// jsonToFile takes a gjson Result object and writes it to a file.
func (c *Cache) jsonToFile(filename string, gj gjson.Result) (err error) {
	jBytes, err := json.MarshalIndent(gj.Value(), "", "  ")
	if err != nil {
		return
	}
	err = os.WriteFile(filename, jBytes, 0644)
	return
}

// timestamp returns a string representation of the current time in ISO 8601 format.
func timestamp() string {
	t := time.Now()
	return t.Format(iso8601)
}

// timeEpoch returns an ISO8601 string representation of an given epoch time.
func timeEpoch(epoch int64) string {
	t := time.Unix(epoch, 0)
	return t.Format(iso8601)
}
