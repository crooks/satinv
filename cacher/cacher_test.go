package cacher

import (
	"errors"
	"log"
	"os"
	"path"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func mkTempDir() string {
	tempDir, err := os.MkdirTemp("/tmp", "sat")
	if err != nil {
		log.Fatalf("Unable to create TempDir: %v", err)
	}
	return tempDir
}

func TestCacher(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	cacheDir := path.Join(tempDir, "cacheDir")
	// The Cache Dir is created by the NewCacher constructor.  It shouldn't exist yet.
	if _, err := os.Stat(cacheDir); err == nil {
		t.Errorf("%s: Cache Dir exists before NewCacher constructor runs", cacheDir)
	}
	c := NewCacher(cacheDir)
	if c.cacheDir != cacheDir {
		t.Errorf("Unexpected cacheDir.  Expected=%s, Got=%s", tempDir, c.cacheDir)
	}
	if _, err := os.Stat(cacheDir); errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s: Cache Dir does not exist after constructor ran", cacheDir)
	}
}

func TestExpire(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	testURL := "https://fake.url"
	testFile := "testfile.json"
	c.AddURL(testURL, testFile, 2)
	expired, err := c.HasExpired(testURL)
	if err != nil {
		t.Errorf("Failed to check expiry for %s: %v", testURL, err)
	}
	if !expired {
		t.Errorf("%s: New cache item should be expired", testURL)
	}
}

func TestWriteRead(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	testFile := path.Join(tempDir, "testfile.json")
	c := NewCacher(tempDir)
	sample := `{"results": ["a","b","c"]}`
	outJson := gjson.Parse(sample)
	c.jsonToFile(testFile, outJson)
	inJson, err := c.jsonFromFile(testFile)
	if err != nil {
		t.Errorf("Failed to fetch json: %v", err)
	}
	jItem := inJson.Get("results").Array()
	if len(jItem) != 3 {
		t.Errorf("Expected results array of 3 items but got %d", len(jItem))
	}
	if jItem[0].String() != "a" || jItem[1].String() != "b" || jItem[2].String() != "c" {
		t.Errorf("Unexpected json content: %v", jItem)
	}
}

func TestGetURL(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	testURL := "http://fakeurl.fake"
	testFile := "test.json"
	_, err := c.GetURL(testURL)
	if err == nil {
		t.Fatalf("No error returned for non existent cache file")
	}
	c.AddURL(testURL, testFile, 2)
	_, err = c.GetURL(testURL)
	if !errors.Is(err, errAPIInit) {
		t.Fatalf("Error: %v", err)
	}
}

func TestGetFile(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	testItem := "filename.fake"
	testFile := "test.txt"
	testString := "Hello World!"
	f, err := os.Create(path.Join(tempDir, testFile))
	if err != nil {
		t.Errorf("Cannot create test file: %v", err)
	}
	f.WriteString(testString)
	f.Close()
	var testValidity int64 = 2
	c.AddFile(testItem, testFile, testValidity)
	fileString, err := c.GetFile(testItem)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	if string(fileString) != testString {
		t.Errorf("Unexpected file content: Expected=%s, Got=%s", testString, string(fileString))
	}
	item, err := c.getItem(testItem)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	if item.url {
		t.Errorf("item.url should be false when adding a file")
	}
}

func TestAddURL(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	testItem := "http://fakeurl.fake"
	testFile := "test.json"
	var testValidity int64 = 2
	c.AddURL(testItem, testFile, testValidity)
	item, err := c.getItem(testItem)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	fullTestFile := path.Join(tempDir, testFile)
	if item.file != fullTestFile {
		t.Errorf("Unexpected filename: Expected=%s, Got=%s", fullTestFile, item.file)
	}
	if item.validity != testValidity {
		t.Errorf("Unexpected validity period for %s: Expected=%d, Got=%d", testItem, testValidity, item.validity)
	}
	if item.expiry != 0 {
		t.Errorf("%s: Expiry should be 0 for new cacheItem", testItem)
	}
	if !item.url {
		t.Errorf("%s: Adding a new URL should set item.url to True", testItem)
	}
}

func TestExportExpiry(t *testing.T) {
	tempDir := mkTempDir()
	defer os.RemoveAll(tempDir)
	c := NewCacher(tempDir)
	testItem := "http://fakeurl.fake"
	testFile := "test.json"
	var testValidity int64 = 2
	c.AddURL(testItem, testFile, testValidity)
	item, err := c.getItem(testItem)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	// At this point, testItem will have a defined validity period (2 seconds) but the expiry time will be 0 because it's a new item
	if item.expiry != 0 {
		t.Errorf("%s: Expiry should be 0 for new cacheItem", testItem)
	}
	// Resetting the Expiry will set it to now+validity
	err = c.ResetExpire(testItem)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	// item needs to be refreshed as it was created before the ResetExpire
	item, err = c.getItem(testItem)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	// These tests ensure the expiry time is aligned with the specified validity period (following ResetExpire)
	now := time.Now().Unix()
	if item.expiry < now {
		t.Errorf("Expiry time in the past: now=%d, expiry=%d", now, item.expiry)
	}
	if item.expiry > now+item.validity+1 {
		t.Errorf("Expiry seems too far into the future: now=%d, expiry=%d", now, item.expiry)
	}
	if !c.writeExpiry {
		t.Errorf("A cache item was changed but the writeExpiry flag is false")
	}
	c.WriteExpiryFile()

	// Create an empty file for the cache item.  This prevents HasExpired from returning true due to the absense of
	// the file.
	fullTestFile := path.Join(tempDir, testFile)
	emptyFile, err := os.Create(fullTestFile)
	if err != nil {
		log.Fatal(err)
	}
	emptyFile.Close()

	// Create a new Cacher object to reimport expiry data
	d := NewCacher(tempDir)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	d.AddURL(testItem, testFile, testValidity)
	// Test HasExpired.
	expired, err := d.HasExpired(testItem)
	if err != nil {
		t.Errorf("%s: %v", testItem, err)
	}
	// Insufficient time should have passed for the item to have expired
	if expired {
		t.Error("Item cache should not be expired")
	}
}
