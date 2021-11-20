package cacher

import (
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path"
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
	c.jsonToFile("test.json", j)
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
	testURL := "http://fakeurl.fake"
	testFile := "test.json"
	c.AddURL(testURL, testFile)
	err := c.exportExpiry()
	if err != nil {
		t.Fatal(err)
	}
	filename := path.Join(tempDir, cacheExpiryFile)
	gj, err := c.jsonFromFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	testExpire := gj.Get("urls.testURL").Int()
	if !gj.Exists() {
		t.Fatal("Expected urls key does not exist")
	}
	if testExpire != 0 {
		t.Fatalf("Unexpected expiry value.  Expected=0, Got=%d", testExpire)
	}
}
