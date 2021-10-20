package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/crooks/satinv/cacher"
	"github.com/crooks/satinv/cidrs"
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

// satTimestamp parses a DateTime string of the format used in the Satellite API
func satTimestamp(ts string) (t time.Time, err error) {
	layout := "2006-01-02 15:04:05 MST"
	t, err = time.Parse(layout, ts)
	if err != nil {
		log.Printf("Sat time parse: %v", err)
		return
	}
	return
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

func importCIDRs() cidrs.Cidrs {
	cidr := make(cidrs.Cidrs)
	cidr.AddCIDRMap(cfg.CIDRs)
	return cidr
}

// mkInventory assembles all the components of a Dynamic Inventory and writes them to Stdout (or a file).
func mkInventory() {
	// Initialize the URL cache
	cache := cacher.NewCacher(cfg.CacheDir)
	cache.SetCacheDuration(cfg.CacheValidity)
	cache.InitAPI(cfg.APIUser, cfg.APIPassword, cfg.APICertFile)
	if flags.Refresh {
		// Force a cache refresh
		cache.SetRefresh()
	}

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
	ans.parseHosts(hosts)
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

// parseHosts creates the inventory hostvars metadata for each host
func (ans *ansible) parseHosts(hosts gjson.Result) {
	defer timeTrack(time.Now(), "parseHosts")
	var err error

	// Import the CIDRs we want to test each address against.
	cidr := importCIDRs()
	if len(cidr) == 0 {
		log.Print("Bypassing CIDR membership processing.  No CIDRs defined.")
	}

	// Initialize the sat_valid inventory group
	// satValidAppend is a special string used by sjson to append entries to an inventory group
	satValidAppend := fmt.Sprintf("%svalid.hosts.-1", cfg.InventoryPrefix)
	// Add sat_valid to the all{children} array
	ans.json, err = sjson.Set(ans.json, "all.children.-1", cfg.InventoryPrefix+"valid")
	if err != nil {
		log.Fatal(err)
	}

	// Iterate through each host in the Satellite results
	for _, h := range hosts.Get("results").Array() {
		// Every individual host map should contain a "name" key
		if !h.Get("name").Exists() {
			log.Print("No hostname found in Satellite host map")
			continue
		}
		hostNameShort := shortName(h.Get("name").String())
		key := fmt.Sprintf("_meta.hostvars.%s", hostNameShort)
		ans.json, err = sjson.Set(ans.json, key, h.Value())
		if err != nil {
			log.Fatal(err)
		}
		ans.hgSatValid(h, satValidAppend, hostNameShort)
		if len(cidr) > 0 {
			ans.hgCIDRMembers(h, cidr)
		}
	}
}

// parseHostCollections iterates through the Satellite Host Collections and associates hostnames with the each
// Collection's host_ids.
func (ans *ansible) parseHostCollections(hosts gjson.Result, cache *cacher.Cache) {
	defer timeTrack(time.Now(), "parseHostCollections")
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
			ans.json, err = sjson.Set(ans.json, collectionAppend, shortName(host))
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

// satValid creates an inventory group of hosts the meet "valid" conditions.
func (ans *ansible) hgSatValid(host gjson.Result, satValidAppend, hostNameShort string) {
	// Check the host has an IP address
	ip4 := host.Get("ip")
	ip6 := host.Get("ip6")
	if !ip4.Exists() && !ip6.Exists() {
		log.Printf("satValid: No IP address found for %s", hostNameShort)
		return
	}
	if ip4.String() == "" && ip6.String() == "" {
		log.Printf("satValid: No valid IP addresses found for %s", hostNameShort)
		return
	}
	// Ensure the host has a valid subscription
	subStatus := host.Get("subscription_status")
	if !subStatus.Exists() || subStatus.Int() != 0 {
		log.Printf("satValid: subscription_status not found for %s", hostNameShort)
		return
	}
	if subStatus.Int() != 0 {
		log.Printf("satValid: Invalid subscription status (%d) for %s", subStatus.Int(), hostNameShort)
		return
	}

	// Check last_checkin date
	checkin := host.Get("subscription_facet_attributes.last_checkin")
	if !checkin.Exists() {
		log.Printf("satValid: subscription_facet_attributes.last_checkin not found for %s", hostNameShort)
		return
	}
	satTime, err := satTimestamp(checkin.String())
	if err != nil {
		// consider the host to be invalid
		log.Printf("satValid: Invalid date/time %s for %s", checkin.String(), hostNameShort)
		return
	}
	oldestValidTime := time.Now().Add(-time.Hour * 24 * time.Duration(cfg.SatValidDays))
	if satTime.Before(oldestValidTime) {
		log.Printf("satValid: Last checkin for %s is too old", hostNameShort)
		return
	}

	// All the above conditions passed; this is a valid host.
	ans.json, err = sjson.Set(ans.json, satValidAppend, hostNameShort)
	if err != nil {
		log.Fatal(err)
	}
}

// hgCIDRMembers compares the IPv4 address of the current host to a list of CIDRs.  When the address is a member of a
// CIDR, its appended to an inventory group for that CIDR.
func (ans *ansible) hgCIDRMembers(host gjson.Result, cidr cidrs.Cidrs) {
	hostNameShort := shortName(host.Get("name").String())

	// Test the validity of the address for CIDR membership processing.
	gjIP4 := host.Get("ip")
	if !gjIP4.Exists() {
		return
	}
	ip4 := gjIP4.String()
	if ip4 == "" {
		return
	}

	// invGrps will contain a slice of all inventory groups the address is a member of.
	invGrps := cidr.ParseCIDRs(ip4)

	var err error
	for _, invGrp := range invGrps {
		sjKey := fmt.Sprintf("%s.hosts.-1", mkInventoryName(invGrp))
		ans.json, err = sjson.Set(ans.json, sjKey, hostNameShort)
		if err != nil {
			log.Printf("hgCIDRMembers: %s: %v", hostNameShort, err)
		}
	}
}

// timeTrack can be used to time the processing duration of a function.
func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s took %s", name, elapsed)
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
