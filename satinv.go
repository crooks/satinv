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

const (
	inventoryName string = "inventory"
)

var (
	cfg   *config.Config
	flags *config.Flags
)

type inventory struct {
	json  string
	cache *cacher.Cache
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
func (inv *inventory) getHostCollection(id string) (gjson.Result, error) {
	collectionURL := fmt.Sprintf("%s/katello/api/host_collections/%s", cfg.API.BaseURL, id)
	collectionFilename := fmt.Sprintf("host_collections_%s.json", id)
	inv.cache.AddURL(collectionURL, collectionFilename)
	collection, err := inv.cache.GetURL(collectionURL)
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

// importCIDRs constructs a new instance of Cidrs and then populates it from a map in the Config.
func importCIDRs() cidrs.Cidrs {
	cidr := make(cidrs.Cidrs)
	cidr.AddCIDRMap(cfg.CIDRs)
	return cidr
}

// refreshInventory produces a new inventory.json copy from the Satellite API (or cache).
func (inv *inventory) refreshInventory() {
	// If URLs have to be pulled from an API, this has to be initialised.
	inv.cache.InitAPI(cfg.API.User, cfg.API.Password, cfg.API.CertFile)

	// Populate the hosts object
	hostsURL := fmt.Sprintf("%s/api/v2/hosts?per_page=1000", cfg.API.BaseURL)
	inv.cache.AddURL(hostsURL, "hosts.json")
	hosts, err := inv.cache.GetURL(hostsURL)
	if err != nil {
		log.Fatalf("Unable to read hosts from JSON file: %v", err)
	}

	// Initialize the inventory object that contains the json string field
	inv.json = "{}"
	inv.json, err = sjson.Set(inv.json, "_meta", "hostvars")
	if err != nil {
		log.Fatal(err)
	}
	inv.parseHosts(hosts)
	inv.parseHostCollections(hosts)
	// For human readability, put an LF on the end of the json.
	inv.json += "\n"
	filename := inv.cache.GetFilename(inventoryName)
	err = ioutil.WriteFile(filename, []byte(inv.json), 0644)
	if err != nil {
		log.Fatalf("WriteFile: %v", err)
	}
	// If the inventory has been successfully refreshed, update the expiry file with a new refresh timestamp.
	// TODO: Make the inventory cache duration configurable
	inv.cache.UpdateExpiry(inventoryName, 3600)
}

// mkInventory assembles all the components of a Dynamic Inventory and writes them to Stdout (or a file).
func mkInventory() {
	// Initialize an inventory struct
	inv := new(inventory)
	// Initialize the URL cache
	inv.cache = cacher.NewCacher(cfg.Cache.Dir)
	// When this function completes, write the expiry file (if one or more cache items have been refreshed).
	defer inv.cache.WriteExpiryFile()
	inv.cache.SetCacheDuration(cfg.Cache.Validity)
	if flags.Refresh {
		// Force a cache refresh
		inv.cache.SetRefresh()
	}

	// This isn't a real URL; it never gets pulled from an API.  It contains the output inventory JSON and enables it
	// to be cached.
	inv.cache.AddURL(inventoryName, fmt.Sprintf("%s.json", inventoryName))
	refresh, err := inv.cache.HasExpired(inventoryName)
	if err != nil {
		log.Fatal(err)
	}
	if refresh {
		inv.refreshInventory()
	} else {
		inv.json = string(inv.cache.GetFile(inventoryName))
	}
	if flags.List {
		_, err = fmt.Fprint(os.Stdout, inv.json)
		if err != nil {
			log.Fatalf("Fprintf: %v", err)
		}
	}
}

// parseHosts creates the inventory hostvars metadata for each host
func (inv *inventory) parseHosts(hosts gjson.Result) {
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
	inv.json, err = sjson.Set(inv.json, "all.children.-1", cfg.InventoryPrefix+"valid")
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
		inv.json, err = sjson.Set(inv.json, key, h.Value())
		if err != nil {
			log.Fatal(err)
		}
		inv.hgSatValid(h, satValidAppend, hostNameShort)
		if len(cidr) > 0 {
			inv.hgCIDRMembers(h, cidr)
		}
	}
}

// parseHostCollections iterates through the Satellite Host Collections and associates hostnames with the each
// Collection's host_ids.
func (inv *inventory) parseHostCollections(hosts gjson.Result) {
	defer timeTrack(time.Now(), "parseHostCollections")
	collectionsURL := fmt.Sprintf("%s/katello/api/host_collections", cfg.API.BaseURL)
	inv.cache.AddURL(collectionsURL, "host_collections.json")
	collections, err := inv.cache.GetURL(collectionsURL)
	if err != nil {
		log.Fatalf("Unable to read JSON from file: %v", err)
	}
	for _, c := range collections.Get("results").Array() {
		hostCollectionName := c.Get("name").String()
		hostCollectionID := c.Get("id").String()
		hostCollection, err := inv.getHostCollection(hostCollectionID)
		if err != nil {
			log.Printf("Unable to get host_collection: %v", err)
			continue
		}
		collectionKey := mkInventoryName(hostCollectionName)
		inv.json, err = sjson.Set(inv.json, "all.children.-1", collectionKey)
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
			inv.json, err = sjson.Set(inv.json, collectionAppend, shortName(host))
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

// satValid creates an inventory group of hosts the meet "valid" conditions.
func (inv *inventory) hgSatValid(host gjson.Result, satValidAppend, hostNameShort string) {
	// Test if the host is excluded in the Config file
	if cfg.SatValidExclude(hostNameShort) {
		log.Printf("SatValid: Host %s is excluded", hostNameShort)
		return
	}
	// Check the host has a valid Operating System installed
	osid := host.Get("operatingsystem_id")
	if !osid.Exists() || osid.Int() == 0 {
		log.Printf("satValid: No valid OS found for %s", hostNameShort)
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
	oldestValidTime := time.Now().Add(-time.Hour * 24 * time.Duration(cfg.ValidDays))
	if satTime.Before(oldestValidTime) {
		log.Printf("satValid: Last checkin for %s is too old", hostNameShort)
		return
	}

	// All the above conditions passed; this is a valid host.
	inv.json, err = sjson.Set(inv.json, satValidAppend, hostNameShort)
	if err != nil {
		log.Fatal(err)
	}
}

// hgCIDRMembers compares the IPv4 address of the current host to a list of CIDRs.  When the address is a member of a
// CIDR, its appended to an inventory group for that CIDR.
func (inv *inventory) hgCIDRMembers(host gjson.Result, cidr cidrs.Cidrs) {
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
		inv.json, err = sjson.Set(inv.json, sjKey, hostNameShort)
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
	// For normal use, log output needs to be surpressed or inventory's dynamic inventory will try to process it.
	if !flags.Debug {
		log.SetOutput(ioutil.Discard)
	}
	cfg, err = config.ParseConfig(flags.Config)
	if err != nil {
		log.Fatalf("Cannot parse config: %v", err)
	}

	mkInventory()
}
