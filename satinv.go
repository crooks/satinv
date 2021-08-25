package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	"github.com/crooks/satinv/config"
	"github.com/tidwall/gjson"
)

// contains returns true if slice contains str.
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// jsonFromFile takes the filename for a file containing json formatted content
// and returns a gjson Result of the file content.
func jsonFromFile(filename string) (gjson.Result, error) {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return gjson.Result{}, err
	}
	return gjson.ParseBytes(b), nil
}

// shortName take a hostname string and returns the shortname for it.
func shortName(host string) string {
	return strings.Split(host, ".")[0]
}

// compareToSat takes a slice of known target hosts and compares it to defined Satellite hosts.  It returns a slice of
// hosts that are in Satellite but are not Prometheus targets.
func compareToSat(targets []string) (misses []string) {
	j, err := jsonFromFile("/home/crooks/sample_json/rhsat.json")
	if err != nil {
		log.Fatalf("Unable to parse json file: %v", err)
	}
	for _, v := range j.Get("results").Array() {
		host := v.Get("name")
		ip := v.Get("ip")
		subscription := v.Get("subscription_status")
		if host.Exists() && subscription.Exists() && ip.Exists() {
			// To be considered valid, a Satellite host must have a subscription and an IP address.
			if subscription.Int() == 0 && ip.String() != "" {
				short := shortName(host.String())
				if !contains(targets, short) {
					misses = append(misses, short)
				}
			}
		}
	}
	return
}

var (
	cfg *config.Config
)

func getHostByID(hosts gjson.Result, id string) (string, error) {
	for _, host := range hosts.Get("results").Array() {
		hostID := host.Get("id")
		if hostID.Exists() && host.Get("id").String() == id {
			name := host.Get("name")
			if name.Exists() {
				return name.String(), nil
			} else {
				err := fmt.Errorf("name not found for id: %s", id)
				return "", err
			}
		}
	}
	err := fmt.Errorf("%s: id not found", id)
	return "", err
}

func mkInventoryName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = cfg.InventoryPrefix + s
	return s
}

func main() {
	var err error
	flags := config.ParseFlags()
	cfg, err = config.ParseConfig(flags.Config)
	if err != nil {
		log.Fatalf("Cannot parse config: %v", err)
	}
	/*
		hosts, err := jsonFromFile("/home/crooks/tmp/rhsat.json")
		if err != nil {
			log.Fatalf("Unable to read JSON from file: %v", err)
		}
		host, err := getHostByID(hosts, "251")
		if err != nil {
			log.Fatalf("Cannot fetch host by ID: %v", err)
		}
		fmt.Println(host)
	*/
	collections, err := jsonFromFile("/home/crooks/tmp/host_collections.json")
	if err != nil {
		log.Fatalf("Unable to read JSON from file: %v", err)
	}
	for _, c := range collections.Get("results").Array() {
		inventoryGroup := mkInventoryName(c.Get("name").String())
		fmt.Println(c.Get("id").String(), mkInventoryName(inventoryGroup))
	}
}
