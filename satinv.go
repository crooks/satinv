package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/crooks/satinv/cacher"
	"github.com/crooks/satinv/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var (
	cfg   *config.Config
	flags *config.Flags
)

type ansible struct {
	json string
}

// shortName take a hostname string and returns the shortname for it.
func shortName(host string) string {
	return strings.Split(host, ".")[0]
}

// getHostByID returns the string representation of a hostname for a given ID string.
func getHostByID(hosts gjson.Result, id string) (string, error) {
	query := fmt.Sprintf("results.#(id=\"%s\").name", id)
	hostname := hosts.Get(query)
	if hostname.Exists() {
		return hostname.String(), nil
	}
	err := fmt.Errorf("name not found for id: %s", id)
	return "", err
}

// mkInventoryName converts a Host Collection name to something compatible with Ansible Inventories.
func mkInventoryName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = cfg.InventoryPrefix + s
	return s
}

// getHostCollection takes an ID string and returns the Host Collection associated with it.
func getHostCollection(id string, cache *cacher.Cache) (gjson.Result, error) {
	collectionURL := fmt.Sprintf("%s/katello/api/host_collections/%s", cfg.APIBaseURL, id)
	collectionFilename := fmt.Sprintf("host_collections_%s.json", id)
	cache.AddURL(collectionURL, collectionFilename)
	collection, err := cache.GetURL(collectionURL)
	if err != nil {
		return gjson.Result{}, err
	}
	collectionID := collection.Get("id")
	if !collectionID.Exists() {
		err := errors.New("host collection has no ID field")
		return gjson.Result{}, err
	}
	if collectionID.String() != id {
		err := errors.New("host collection ID does not match requested ID")
		return gjson.Result{}, err
	}
	return collection, nil
}

// mkInventory assembles all the components of a Dynamic Inventory and writes them to Stdout (or a file).
func mkInventory() {
	// Initialize the URL cache
	cache := cacher.NewCacher(cfg.CacheDIR)
	cache.SetCacheDuration(cfg.CacheValidity)
	cache.InitAPI(cfg.APIUser, cfg.APIPassword, cfg.APICertFile)

	// Populate the hosts object
	hostsURL := fmt.Sprintf("%s/api/v2/hosts?per_page=1000", cfg.APIBaseURL)
	cache.AddURL(hostsURL, "hosts.json")
	hosts, err := cache.GetURL(hostsURL)
	if err != nil {
		log.Fatalf("Unable to read hosts from JSON file: %v", err)
	}

	// Initialize the ansible object that contains the json string field
	ans := new(ansible)
	ans.json = "{}"
	ans.json, err = sjson.Set(ans.json, "_meta", "hostvars")
	if err != nil {
		log.Fatal(err)
	}
	ans.parseHosts(hosts, cache)
	ans.parseHostCollections(hosts, cache)
	// For human readability, put an LF on the end of the json.
	ans.json += "\n"
	if flags.List {
		_, err = fmt.Fprint(os.Stdout, ans.json)
		if err != nil {
			log.Fatalf("Fprintf: %v", err)
		}
	}
	err = ioutil.WriteFile(cfg.OutJSON, []byte(ans.json), 0644)
	if err != nil {
		log.Fatalf("WriteFile: %v", err)
	}
}

func (ans *ansible) parseHosts(hosts gjson.Result, cache *cacher.Cache) {
	for _, h := range hosts.Get("results.#.name").Array() {
		query := fmt.Sprintf("results.#(name=%s)", h)
		key := fmt.Sprintf("_meta.hostvars.%s", shortName(h.String()))
		var err error
		ans.json, err = sjson.Set(ans.json, key, hosts.Get(query).Value())
		if err != nil {
			log.Fatal(err)
		}
	}
}

// parseHostCollections iterates through the Satellite Host Collections and associates hostnames with the each
// Collection's host_ids.
func (ans *ansible) parseHostCollections(hosts gjson.Result, cache *cacher.Cache) {
	collectionsURL := fmt.Sprintf("%s/katello/api/host_collections", cfg.APIBaseURL)
	cache.AddURL(collectionsURL, "host_collections.json")
	collections, err := cache.GetURL(collectionsURL)
	if err != nil {
		log.Fatalf("Unable to read JSON from file: %v", err)
	}
	for _, c := range collections.Get("results").Array() {
		hostCollectionName := c.Get("name").String()
		hostCollectionID := c.Get("id").String()
		hostCollection, err := getHostCollection(hostCollectionID, cache)
		if err != nil {
			log.Printf("Unable to get host_collection: %v", err)
			continue
		}
		collectionKey := mkInventoryName(hostCollectionName)
		ans.json, err = sjson.Set(ans.json, "all.children.-1", collectionKey)
		if err != nil {
			log.Fatal(err)
		}
		collectionAppend := fmt.Sprintf("%s.hosts.-1", collectionKey)
		for _, v := range hostCollection.Get("host_ids").Array() {
			host, err := getHostByID(hosts, v.String())
			if err != nil {
				log.Printf("Cannot fetch host by ID: %v", err)
				continue
			}
			ans.json, err = sjson.Set(ans.json, collectionAppend, host)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

func main() {
	var err error
	flags = config.ParseFlags()
	// For normal use, log output needs to be surpressed or ansible's dynamic inventory will try to process it.
	if !flags.Debug {
		log.SetOutput(ioutil.Discard)
	}
	cfg, err = config.ParseConfig(flags.Config)
	if err != nil {
		log.Fatalf("Cannot parse config: %v", err)
	}

	mkInventory()
}
