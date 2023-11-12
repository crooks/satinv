package config

import (
	"os"
	"os/user"
	"path"
	"testing"
)

// containsStr returns True if a given string is a member of a given slice
func containsStr(str string, strs []string) bool {
	for _, s := range strs {
		if s == str {
			return true
		}
	}
	return false
}

func TestFlags(t *testing.T) {
	expectedConfig := "/etc/promsat/fake.yml"
	// This needs to be set prior to doing ParseFlags()
	os.Setenv("SATINVCFG", expectedConfig)
	f := ParseFlags()
	if f.Config != expectedConfig {
		t.Fatalf("Expected --config to contain \"%v\" but got \"%v\".", expectedConfig, f.Config)
	}
}

func TestConfig(t *testing.T) {
	testFile, err := os.CreateTemp("", "testcfg")
	if err != nil {
		t.Fatalf("Unable to create TempFile: %v", err)
	}
	defer os.Remove(testFile.Name())
	cfgValidExcludeHosts := "cfg_valid_exclude_hosts"
	cfgValidExcludeRegex := "cfg_valid_exclude_regex"
	fakeCfg := new(Config)
	fakeCfg.Valid.Hours = defaultSatValidHours
	fakeCfg.Valid.ExcludeHosts = append(fakeCfg.Valid.ExcludeHosts, cfgValidExcludeHosts)
	fakeCfg.Valid.ExcludeRegex = append(fakeCfg.Valid.ExcludeRegex, cfgValidExcludeRegex)
	fakeCfg.Cache.ValidityHosts = defaultCacheValiditySeconds
	fakeCfg.Cache.ValidityCollections = defaultCacheValiditySeconds
	fakeCfg.Cache.ValidityInventory = defaultInventoryValiditySeconds
	fakeCfg.InventoryPrefix = "sat_"
	fakeCfg.WriteConfig(testFile.Name())

	cfg, err := ParseConfig(testFile.Name())
	if err != nil {
		t.Fatalf("ParseConfig returned: %v", err)
	}

	if cfg.Valid.Hours != fakeCfg.Valid.Hours || cfg.Valid.Hours != defaultSatValidHours {
		t.Fatalf(
			"Unexpected config.Valid.Hours. Default=%d, Expected=%d, Got=%d",
			defaultSatValidHours, fakeCfg.Valid.Hours, cfg.Valid.Hours)
	}
	if !containsStr(cfgValidExcludeHosts, cfg.Valid.ExcludeHosts) {
		t.Errorf("cfg.Valid.ExcludeHosts does not include the string %s", cfgValidExcludeHosts)
	}
	if !containsStr(cfgValidExcludeRegex, cfg.Valid.ExcludeRegex) {
		t.Errorf("cfg.Valid.ExcludeRegex does not include the string %s", cfgValidExcludeRegex)
	}
	// This isn't set in fakeCfg so it should default to False
	if cfg.Valid.Unlicensed {
		t.Error(("cfg.Valid.Unlicensed should be false"))
	}
	if cfg.Cache.ValidityHosts != fakeCfg.Cache.ValidityHosts || cfg.Cache.ValidityHosts != defaultCacheValiditySeconds {
		t.Fatalf(
			"Unexpected config.Cache.Validity. Default=%d, Expected=%d, Got=%d",
			defaultCacheValiditySeconds, fakeCfg.Cache.ValidityHosts, cfg.Cache.ValidityHosts)
	}
	if cfg.Cache.ValidityInventory != fakeCfg.Cache.ValidityInventory || cfg.Cache.ValidityInventory != defaultInventoryValiditySeconds {
		t.Fatalf(
			"Unexpected config.Cache.InventoryValidity. Default=%d, Expected=%d, Got=%d",
			defaultInventoryValiditySeconds, fakeCfg.Cache.ValidityInventory, cfg.Cache.ValidityInventory)
	}
	if cfg.InventoryPrefix != fakeCfg.InventoryPrefix {
		t.Errorf(
			"Unexpected InventoryPrefix. Expected=%s, Got=%s", fakeCfg.InventoryPrefix, cfg.InventoryPrefix)
	}
}

func TestExpandTilde(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatalf("Unable to ascertain current user: %v", err)
	}
	// Test homedir, without path
	testDir := "~"
	expectDir := u.HomeDir
	resultDir := expandTilde(testDir)
	if expectDir != resultDir {
		t.Errorf("Tilde expansion failed.  Expected=%s, Got=%s", expectDir, resultDir)
	}
	// Test homedir with path
	testDir = "~/dir1/dir2/filename"
	expectDir = path.Join(u.HomeDir, "dir1/dir2/filename")
	resultDir = expandTilde(testDir)
	if expectDir != resultDir {
		t.Errorf("Tilde expansion failed.  Expected=%s, Got=%s", expectDir, resultDir)
	}
	// Test path without homedir
	testDir = "/dir1/dir2/filename"
	expectDir = "/dir1/dir2/filename"
	resultDir = expandTilde(testDir)
	if expectDir != resultDir {
		t.Errorf("Tilde expansion failed.  Expected=%s, Got=%s", expectDir, resultDir)
	}
}
