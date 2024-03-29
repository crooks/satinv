package main

import (
	"errors"
	"fmt"
	stdlog "log"
	"os"
	"strings"
	"time"

	"github.com/Masterminds/log-go"
	"github.com/crooks/jlog"
	loglevel "github.com/crooks/log-go-level"
	"github.com/crooks/satinv/cacher"
	"github.com/crooks/satinv/cidrs"
	"github.com/crooks/satinv/config"
	"github.com/crooks/satinv/multire"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	inventoryName string = "inventory"
	shortDate     string = "2006-01-02 15:04:05 MST"
)

var (
	cfg   *config.Config
	flags *config.Flags
)

type inventory struct {
	json            string
	cache           *cacher.Cache
	oldestValidTime time.Time
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
		log.Errorf("Sat time parse: %v", err)
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

// containsStr returns True if a given string is a member of a given slice
func containsStr(str string, strs []string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
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
	inv.cache.AddURL(collectionURL, collectionFilename, cfg.Cache.ValidityCollections)
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
	inv.cache.AddURL(hostsURL, "hosts.json", cfg.Cache.ValidityHosts)
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
	filename, err := inv.cache.GetFilename(inventoryName)
	if err != nil {
		log.Fatalf("Unable to get cached filename: %v", err)
	}
	err = os.WriteFile(filename, []byte(inv.json), 0644)
	if err != nil {
		log.Fatalf("WriteFile: %v", err)
	}
	// If the inventory has been successfully refreshed, update the expiry file with a new refresh timestamp.
	inv.cache.ResetExpire(inventoryName)
}

// mkInventory assembles all the components of a Dynamic Inventory and writes them to Stdout (or a file).
func mkInventory() {
	// Initialize an inventory struct
	inv := new(inventory)
	// Initialize the URL cache
	inv.cache = cacher.NewCacher(cfg.Cache.Dir)
	// When this function completes, write the expiry file (if one or more cache items have been refreshed).
	if flags.Refresh {
		// Force a cache refresh
		inv.cache.SetRefresh()
	}
	// An age in hours beyond which hosts will be considered invalid (excluded from hgValid).
	inv.oldestValidTime = time.Now().Add(-time.Hour * time.Duration(cfg.Valid.Hours))
	log.Debugf("Hosts older then %s will be deemed invalid", inv.oldestValidTime.Format(shortDate))

	// The inventory is the output of the entire process.  We cache it to avoid having to reconstruct it from source APIs.
	inv.cache.AddFile(inventoryName, fmt.Sprintf("%s.json", inventoryName), cfg.Cache.ValidityInventory)
	refresh, err := inv.cache.HasExpired(inventoryName)
	if err != nil {
		log.Fatal(err)
	}
	if refresh {
		log.Debugf("Cache of the %s file has expired.  Refreshing it.", inventoryName)
		inv.refreshInventory()
	} else {
		log.Debugf("Cache of the %s file is still valid so not refreshing it.", inventoryName)
		i, err := inv.cache.GetFile(inventoryName)
		if err != nil {
			log.Fatalf("Unable to get file: %v", err)
		}
		inv.json = string(i)
	}
	if flags.List {
		_, err = fmt.Fprint(os.Stdout, inv.json)
		if err != nil {
			log.Fatalf("Fprintf: %v", err)
		}
	}
	inv.cache.WriteExpiryFile()
}

// parseHosts creates the inventory hostvars metadata for each host
func (inv *inventory) parseHosts(hosts gjson.Result) {
	defer timeTrack(time.Now(), "parseHosts")
	var err error

	// Import the CIDRs we want to test each address against.
	cidr := importCIDRs()
	if len(cidr) == 0 {
		log.Debug("Bypassing CIDR membership processing.  No CIDRs defined.")
	}

	// Initialize the valid inventory group
	// validAppend is a special string used by sjson to append entries to an inventory group
	validAppend := fmt.Sprintf("%svalid.hosts.-1", cfg.InventoryPrefix)
	// Add "valid" to the all{children} array
	inv.json, err = sjson.Set(inv.json, "all.children.-1", cfg.InventoryPrefix+"valid")
	if err != nil {
		log.Fatal(err)
	}

	// Before we get into a hosts loop, create an instance of multiRE to test hostnames against Regular Expressions
	validExcludeRE := multire.InitRegex(cfg.Valid.ExcludeRegex)

	// Iterate through each host in the Satellite results
	for _, h := range hosts.Get("results").Array() {
		// Every individual host map should contain a "name" key
		if !h.Get("name").Exists() {
			log.Errorf("No hostname found in Satellite host map")
			continue
		}
		hostNameShort := shortName(h.Get("name").String())
		log.Debugf("Parsing Satellite info for host: %s", hostNameShort)
		key := fmt.Sprintf("_meta.hostvars.%s", hostNameShort)
		inv.json, err = sjson.Set(inv.json, key, h.Value())
		if err != nil {
			log.Fatal(err)
		}
		inv.hgValid(h, validAppend, hostNameShort, validExcludeRE)
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
	inv.cache.AddURL(collectionsURL, "host_collections.json", cfg.Cache.ValidityCollections)
	collections, err := inv.cache.GetURL(collectionsURL)
	if err != nil {
		log.Fatalf("Unable to read JSON from file: %v", err)
	}
	for _, c := range collections.Get("results").Array() {
		hostCollectionName := c.Get("name").String()
		hostCollectionID := c.Get("id").String()
		log.Debugf("Parsing Satellite Host Collection. Name=%s, ID=%s", hostCollectionName, hostCollectionID)
		hostCollection, err := inv.getHostCollection(hostCollectionID)
		if err != nil {
			log.Warnf("Unable to get host_collection: %v", err)
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
				log.Warnf("Cannot fetch host by ID: %v", err)
				continue
			}
			inv.json, err = sjson.Set(inv.json, collectionAppend, shortName(host))
			if err != nil {
				log.Fatal(err)
			}
		}
	}
}

// hgValid creates an inventory group of hosts that meet "valid" conditions.
func (inv *inventory) hgValid(host gjson.Result, validAppend, hostNameShort string, validExcludeRE multire.MultiRE) {
	// Test if the host is excluded in the Config file
	if containsStr(hostNameShort, cfg.Valid.ExcludeHosts) {
		log.Infof("%svalid: Host %s is excluded from inventory group", cfg.InventoryPrefix, hostNameShort)
		return
	}
	// Test if the host is excluded by regex matching the hostname
	if validExcludeRE.Match(hostNameShort) {
		log.Infof("%svalid: Host %s is excluded from inventory group by Regular Expression match", cfg.InventoryPrefix, hostNameShort)
		return
	}
	// Check the host has a valid Operating System installed
	osid := host.Get("operatingsystem_id")
	if !osid.Exists() || osid.Int() == 0 {
		log.Debugf("%svalid: No valid OS found for %s", cfg.InventoryPrefix, hostNameShort)
		return
	}
	// Ensure the host has a valid subscription
	subStatus := host.Get("subscription_status")
	if !subStatus.Exists() {
		log.Warnf("%svalid: subscription_status not found for %s", cfg.InventoryPrefix, hostNameShort)
		return
	}
	if subStatus.Int() != 0 && !cfg.Valid.Unlicensed {
		log.Infof("%svalid: Invalid subscription status (%d) for %s", cfg.InventoryPrefix, subStatus.Int(), hostNameShort)
		return
	}

	// Check last_checkin date
	checkin := host.Get("subscription_facet_attributes.last_checkin")
	if !checkin.Exists() {
		log.Warnf("%svalid: subscription_facet_attributes.last_checkin not found for %s", cfg.InventoryPrefix, hostNameShort)
		return
	}
	satTime, err := satTimestamp(checkin.String())
	if err != nil {
		// consider the host to be invalid
		log.Warnf("%svalid: Cannot parse date/time string %s for host %s", cfg.InventoryPrefix, checkin.String(), hostNameShort)
		return
	}
	if satTime.Before(inv.oldestValidTime) {
		log.Infof("Last checkin for %s is too old. Excluding from %s_valid.", hostNameShort, cfg.InventoryPrefix)
		return
	}

	// All the above conditions passed; this is a valid host.
	inv.json, err = sjson.Set(inv.json, validAppend, hostNameShort)
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
			log.Warnf("hgCIDRMembers: %s: %v", hostNameShort, err)
		}
	}
}

// timeTrack can be used to time the processing duration of a function.
func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Infof("%s took %s", name, elapsed)
}

func main() {
	var err error
	flags = config.ParseFlags()
	cfg, err = config.ParseConfig(flags.Config)
	if err != nil {
		log.Fatalf("Cannot parse config: %v", err)
	}
	loglev, err := loglevel.ParseLevel(cfg.Logging.LevelStr)
	if err != nil {
		log.Fatalf("Unable to set log level: %v", err)
	}
	if cfg.Logging.Journal && !jlog.Enabled() {
		log.Warn("Cannot log to systemd journal")
	}
	if cfg.Logging.Journal && jlog.Enabled() {
		log.Current = jlog.NewJournal(loglev)
		log.Debugf("Logging to journal has been initialised at level: %s", cfg.Logging.LevelStr)
	} else {
		if cfg.Logging.Filename == "" {
			log.Fatal("Cannot log to file, no filename specified in config")
		}
		logWriter, err := os.OpenFile(cfg.Logging.Filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("Unable to open logfile: %s", err)
		}
		defer logWriter.Close()
		stdlog.SetOutput(logWriter)
		log.Current = log.StdLogger{Level: loglev}
		log.Debugf("Logging to file %s has been initialised at level: %s", cfg.Logging.Filename, cfg.Logging.LevelStr)
	}
	// Time to do some real work
	mkInventory()
}
