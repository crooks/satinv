// cacher provides disk caching of json retrieved from APIs
package cacher

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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

type Cache struct {
	api          *satapi.AuthClient
	apiInit      bool // Test if the API has been initialised
	cacheDir     string
	cacheExpiry  map[string]int64  // k=url, v=epochtime
	cacheFiles   map[string]string // k=url, v=cacheFile
	cachePeriod  int64             // Seconds
	cacheRefresh bool              // Ignore the cache and grab new URLs
	writeExpiry  bool              // Write expiry data to disk
}

func NewCacher(cacheDir string) *Cache {
	c := new(Cache)
	c.cacheDir = cacheDir
	log.Printf("Cache dir set to: %s", c.cacheDir)
	c.cacheFiles = make(map[string]string)
	c.cacheExpiry = make(map[string]int64)
	// This is the only time the expire JSON is read from file.  After this, it resides in memory and only gets written
	// to file.  If the read fails, the Cache is assumed to be empty.
	c.importExpiry()
	return c
}

func (c *Cache) GetFilename(item string) (filename string) {
	var ok bool
	if filename, ok = c.cacheFiles[item]; !ok {
		log.Fatalf("No cache file associated with item \"%s\"", item)
	}
	return
}

func (c *Cache) InitAPI(username, password, cert string) {
	c.api = satapi.NewBasicAuthClient(username, password, cert)
	c.apiInit = true
}

// SetRefresh instructs GetURL to ignore cached files
func (c *Cache) SetRefresh() {
	c.cacheRefresh = true
	log.Print("Forcing cache refresh")
}

func (c *Cache) HasExpired(apiURL string) (refresh bool, err error) {
	var fileName string
	var ok bool
	var expire int64
	// Test if the cacheFiles map contains the URL
	if fileName, ok = c.cacheFiles[apiURL]; !ok {
		err = fmt.Errorf("no cache file associated with %s", apiURL)
		return
	}
	if c.cacheRefresh {
		// Instructed to force a refresh
		log.Printf("Forced refresh of %s", apiURL)
		refresh = true
	} else if _, existErr := os.Stat(fileName); os.IsNotExist(existErr) {
		// File associated with the URL doesn't exist
		log.Printf("Cache file %s for URL %s does not exist", fileName, apiURL)
		refresh = true
	} else if expire, ok = c.cacheExpiry[apiURL]; !ok {
		// There should always be an expiry entry associated with a URL because the addURL function creates it. Despite
		// this, we'll attempt to fetch the URL and create an expiry key for it.
		log.Printf("No Cache expiry data for URL: %s", apiURL)
		refresh = true
	} else if time.Now().Unix() > expire {
		// The Cache entry has expired
		log.Printf("Cache for %s has expired", apiURL)
		refresh = true
	} else {
		refresh = false
	}
	return
}

// AddURL registers a URL with a filename to contain its cached data.  If the URL has no expiry associated with it, a
// new entry is created in the expiry cache and immediately set to expired.
func (c *Cache) AddURL(apiURL, fileName string) {
	c.cacheFiles[apiURL] = path.Join(c.cacheDir, fileName)
	if _, ok := c.cacheExpiry[apiURL]; !ok {
		// Create an expiry key for the URL and expire it.
		log.Printf("No Cache entry for %s.  Adding a new one.", apiURL)
		c.cacheExpiry[apiURL] = 0
	}
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
			c.cacheExpiry[k] = epochExpiry
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
	sj, err = sjson.Set(sj, "urls", c.cacheExpiry)
	if err != nil {
		return err
	}
	// Add a LF to the end of the file
	sj += "\n"
	fileName := path.Join(c.cacheDir, cacheExpiryFile)
	err = ioutil.WriteFile(fileName, []byte(sj), 0644)
	if err != nil {
		return err
	}
	log.Printf("Expiry cache written to: %s", fileName)
	return nil
}

// UpdateExpiry revises the expiry time of a given cache item.
func (c *Cache) UpdateExpiry(item string, period int64) (newExpire int64) {
	newExpire = expireTime(period)
	c.cacheExpiry[item] = newExpire
	c.writeExpiry = true
	return
}

func (c *Cache) WriteExpiryFile() {
	if c.writeExpiry {
		c.exportExpiry()
	}
}

// getURLFromAPI is called when a cache item has expired and a new copy needs to be grabbed from the API.
func (c *Cache) getURLFromAPI(apiURL string) (gj gjson.Result, err error) {
	if !c.apiInit {
		err = errAPIInit
		return
	}
	log.Printf("Requested retreival of: %s", apiURL)
	bytes, err := c.api.GetJSON(apiURL)
	if err != nil {
		err = fmt.Errorf("unable to parse %s: %v", apiURL, err)
		return
	}
	gj = gjson.ParseBytes(bytes)
	err = c.jsonToFile(c.cacheFiles[apiURL], gj)
	if err != nil {
		err = fmt.Errorf("unable to read JSON: %v", err)
		return
	}
	// We have successfully retreived a URL so update its cache expiry time.
	c.UpdateExpiry(apiURL, c.cachePeriod)
	return
}

// GetURL returns the file content associated with a cache key.  If the cache has expired, the content will instead be
// grabbed from the API.
func (c *Cache) GetURL(apiURL string) (gj gjson.Result, err error) {
	refresh, err := c.HasExpired(apiURL)
	if err != nil {
		return
	}
	if refresh {
		gj, err = c.getURLFromAPI(apiURL)
		return
	}
	// Try and get the requested json from the Cache File
	gj, err = c.jsonFromFile(c.cacheFiles[apiURL])
	if err != nil {
		// Failed to read the Cache File, get it from the API instead
		gj, err = c.getURLFromAPI(apiURL)
	}
	return
}

// GetFile reads a cache item's file from disk and returns it as a byte slice.
func (c *Cache) GetFile(item string) []byte {
	b, err := ioutil.ReadFile(c.cacheFiles[item])
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
	b, err := ioutil.ReadFile(filename)
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
	err = ioutil.WriteFile(filename, jBytes, 0644)
	return
}

// timestamp returns a string representation of the current time in ISO 8601 format.
func timestamp() string {
	t := time.Now()
	iso8601 := "2006-01-02T15:04:05Z"
	return t.Format(iso8601)
}

func timeEpoch(epoch int64) string {
	t := time.Unix(epoch, 0)
	return t.Format(iso8601)
}
