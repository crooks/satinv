package cacher

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func mkTempDir() string {
	tempDir, err := ioutil.TempDir("/tmp", "sat")
	if err != nil {
		log.Fatalf("Unable to create TempDir: %v", err)
	}
	return tempDir
}

// abs implements the standard abs function
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestCacher(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	if c.cacheDir != tempDir {
		t.Fatalf("Unexpected cacheDir.  Expected=%s, Got=%s", tempDir, c.cacheDir)
	}
}

func TestExpire(t *testing.T) {
	c := NewCacher("fake")
	var durationSecs int64 = 2
	c.SetCacheDuration(durationSecs)
	now := time.Now().Unix()
	expire := c.expireTime()
	if abs(expire-(now+durationSecs)) > 2 {
		t.Fatalf("Unexpected cachePeriod.  Expected=%d, Got=%d", c.expireTime(), now)
	}
	if time.Now().Unix() > expire {
		t.Fatal("Cache should not be expired")
	}
	time.Sleep(time.Second * time.Duration(3))
	if time.Now().Unix() < expire {
		t.Fatal("Cache should be expired")
	}
}

func TestWrite(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	sample := `{"results": ["a","b","c"]}`
	j := gjson.Parse(sample)
	c.jsonToFile(path.Join(tempDir, "test.json"), j)
}

func TestGetURL(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	c.SetCacheDuration(60)
	testURL := "http://fakeurl.fake"
	testFile := "test.json"
	_, err := c.GetURL(testURL)
	if err == nil {
		t.Fatalf("No error returned for non existent cache file")
	}
	c.AddURL(testURL, testFile)
	_, err = c.GetURL(testURL)
	if !errors.Is(err, errAPIInit) {
		t.Fatalf("Error: %v", err)
	}
}

func TestExportExpiry(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	testItem := "http://fakeurl.fake"
	testFile := "test.json"
	c.AddURL(testItem, testFile)
	// UpdateExpiry also writes the JSON to the test.json file.
	testExpire, err := c.UpdateExpiry(testItem)
	if err != nil {
		t.Fatal(err)
	}
	// Create an empty file for the cache item.  This prevents HasExpired from returning true due to the absense of
	// the file.
	fullTestFile := path.Join(tempDir, testFile)
	emptyFile, err := os.Create(fullTestFile)
	if err != nil {
		log.Fatal(err)
	}
	emptyFile.Close()

	// Test the cache item filename matches testFile
	if c.GetFilename(testItem) != fullTestFile {
		t.Errorf("Unexpected filename: Expected=%s, Got=%s", fullTestFile, c.GetFilename(testItem))
	}

	// Fetch the JSON from tempfile
	filename := path.Join(tempDir, cacheExpiryFile)
	gj, err := c.jsonFromFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	// For gjson, the dot needs to be escaped.
	gjSafeItem := strings.Replace(testItem, ".", "\\.", -1)
	gjItem := fmt.Sprintf("urls.%s", gjSafeItem)
	//getURL := "urls.http://fakeurl\\.fake"
	getItem := gj.Get(gjItem)
	if !getItem.Exists() {
		t.Errorf("Expected url key does not exist: %s", testItem)
	}
	getExpire := getItem.Int()
	if getExpire != testExpire {
		t.Fatalf("Unexpected expiry value.  Expected=%d, Got=%d", testExpire, getExpire)
	}

	// Test HasExpired.
	expiry, err := c.HasExpired(testItem)
	if err != nil {
		log.Fatal(err)
	}
	if expiry {
		t.Error("Item cache should not be expired")
	}
	// Reset the item cache expiry to epoch zero (expired)
	c.cacheExpiry[testItem] = 0
	expiry, err = c.HasExpired(testItem)
	if err != nil {
		log.Fatal(err)
	}
	if !expiry {
		t.Error("Item cache should be expired")
	}
}
