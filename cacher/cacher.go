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
	errNoItem  = errors.New("requested item not in content cache")
)

// Item contains variables relating to each item stored in the cache
type Item struct {
	url      bool   // If it's not a URL, it's a file
	expiry   int64  // Epoch expiry time
	file     string // Filename associated with the cached content
	validity int64  // Validity period in seconds
}

type Cache struct {
	api          *satapi.AuthClient
	apiInit      bool // Test if the API has been initialised
	cacheDir     string
	content      map[string]Item // A cache of Item structs
	cacheRefresh bool            // Ignore the cache and grab new URLs
	writeExpiry  bool            // Write expiry data to disk
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
	c.content = make(map[string]Item)
	// This is the only time the expire JSON is read from file.  After this, it resides in memory and only gets written
	// to file.  If the read fails, the Cache is assumed to be empty.
	c.importExpiry()
	return c
}

// getItem returns a requested item from the content cache
func (c *Cache) getItem(itemKey string) (Item, error) {
	item, ok := c.content[itemKey]
	if !ok {
		return item, errNoItem
	}
	return item, nil
}

// GetFilename returns the filename for a given cache item
func (c *Cache) GetFilename(itemKey string) (string, error) {
	item, err := c.getItem(itemKey)
	if err != nil {
		return "", err
	}
	return item.file, nil
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
	item, err := c.getItem(itemKey)
	if err != nil {
		return
	}
	if c.cacheRefresh {
		// Instructed to force a refresh
		log.Printf("Forced refresh of %s", itemKey)
		refresh = true
	} else if _, existErr := os.Stat(item.file); os.IsNotExist(existErr) {
		// File associated with the URL doesn't exist
		log.Printf("Cache file for URL %s does not exist", itemKey)
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

func (c *Cache) addItem(itemKey string, expireEpoch int64, isURL bool) (err error) {
	item, ok := c.content[itemKey]
	if ok {
		// Cache item already exists.  Why?
		log.Printf("Warning: Cache item %s should not exist", itemKey)
	}
	item.expiry = expireEpoch
	item.url = isURL
	c.content[itemKey] = item
	return
}

// ResetExpire resets the expiry field of a cache Item to current time + the defined validity period
func (c *Cache) ResetExpire(itemKey string) (err error) {
	item, err := c.getItem(itemKey)
	if err != nil {
		return
	}
	item.expiry = time.Now().Unix() + item.validity
	c.content[itemKey] = item
	// Setting WriteExpire indicates the cache file needs to be rewritten (something has changed).
	c.writeExpiry = true
	return
}

// AddURL registers a URL with a filename to contain its cached data.  If the URL has no expiry associated with it, a
// new entry is created in the expiry cache and immediately set to expired.
func (c *Cache) AddURL(itemKey, fileName string, validity int64) {
	item, ok := c.content[itemKey]
	item.url = true
	item.validity = validity
	item.file = path.Join(c.cacheDir, fileName)
	if !ok {
		// If the item was imported from the expiry file, this will already be set
		item.expiry = 0
	}
	c.content[itemKey] = item
}

// AddFile registers a file into the content cache.
func (c *Cache) AddFile(itemKey, fileName string, validity int64) {
	item, ok := c.content[itemKey]
	item.url = false
	item.validity = validity
	item.file = path.Join(c.cacheDir, fileName)
	if !ok {
		item.expiry = 0
	}
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
			c.addItem(k, epochExpiry, true)
		}
	}
	for k, v := range j.Get("files").Map() {
		epochExpiry := v.Int()
		if epochExpiry > ageLimit {
			log.Printf("Importing Cache entry: url=%s, expiry=%s", k, timeEpoch(epochExpiry))
			c.addItem(k, epochExpiry, false)
		}
	}
}

// WriteExpiryFile writes the cache expiry map to a file in JSON format.
func (c *Cache) WriteExpiryFile() error {
	if !c.writeExpiry {
		log.Print("Not writing Expiry File, nothing has changed")
		return nil
	}
	sj, err := sjson.Set("", "write_time", timestamp())
	if err != nil {
		return err
	}
	// expireMap creates a simple map of item->expireTime (epoch)
	expireMapURLs := make(map[string]int64)
	expireMapFiles := make(map[string]int64)
	for k, v := range c.content {
		if v.url {
			expireMapURLs[k] = v.expiry
		} else {
			expireMapFiles[k] = v.expiry
		}
	}
	sj, err = sjson.Set(sj, "urls", expireMapURLs)
	if err != nil {
		return err
	}
	sj, err = sjson.Set(sj, "files", expireMapFiles)
	if err != nil {
		return err
	}
	// Add a LF to the end of the file
	sj += "\n"
	// The cacheDir is defined in NewCacher so it's consistent, all be it real or a tempDir created by Unit Tests.
	filename := path.Join(c.cacheDir, cacheExpiryFile)
	err = os.WriteFile(filename, []byte(sj), 0644)
	if err != nil {
		return err
	}
	log.Printf("Expiry cache written to: %s", filename)
	c.writeExpiry = false
	return nil
}

// getURLFromAPI is called when a cache item has expired and a new copy needs to be grabbed from the API.
func (c *Cache) getURLFromAPI(itemKey string) (gj gjson.Result, err error) {
	if !c.apiInit {
		err = errAPIInit
		return
	}
	log.Printf("Requested retreival of: %s", itemKey)
	bytes, err := c.api.GetJSON(itemKey)
	if err != nil {
		err = fmt.Errorf("unable to parse %s: %v", itemKey, err)
		return
	}
	gj = gjson.ParseBytes(bytes)
	item, ok := c.content[itemKey]
	if !ok {
		err = fmt.Errorf("item %s not in cache content", itemKey)
		return
	}
	err = c.jsonToFile(item.file, gj)
	if err != nil {
		err = fmt.Errorf("unable to read JSON: %v", err)
		return
	}
	// We have successfully retreived a URL so update its cache expiry time.
	c.ResetExpire(itemKey)
	return
}

// GetURL returns the file content associated with a cache key.  If the cache has expired, the content will instead be
// grabbed from the API.
func (c *Cache) GetURL(itemKey string) (gj gjson.Result, err error) {
	item, err := c.getItem(itemKey)
	if err != nil {
		return
	}
	if !item.url {
		// This function is exclusively for URLs
		err = errors.New("requested URL is a file")
		return

	}
	refresh, err := c.HasExpired(itemKey)
	if err != nil {
		return
	}
	if refresh {
		gj, err = c.getURLFromAPI(itemKey)
		return
	}
	// Try and get the requested json from the Cache File
	gj, err = c.jsonFromFile(item.file)
	if err != nil {
		// Failed to read the Cache File, get it from the API instead
		gj, err = c.getURLFromAPI(itemKey)
	}
	return
}

// GetFile reads a cache item's file from disk and returns it as a byte slice.
func (c *Cache) GetFile(itemKey string) (b []byte, err error) {
	item, err := c.getItem(itemKey)
	if err != nil {
		return
	}
	if item.url {
		err = errors.New("requested file is a URL")
		return
	}
	b, err = os.ReadFile(item.file)
	if err != nil {
		return
	}
	return
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
