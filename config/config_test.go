package config

import (
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"testing"
)

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
	testFile, err := ioutil.TempFile("/tmp", "testcfg")
	if err != nil {
		t.Fatalf("Unable to create TempFile: %v", err)
	}
	defer os.Remove(testFile.Name())
	fakeCfg := new(Config)
	fakeCfg.ValidDays = defaultSatValidDays
	fakeCfg.ValidExcludeHosts = append(fakeCfg.ValidExcludeHosts, "svexclude")
	fakeCfg.Cache.Validity = defaultCacheValiditySeconds
	fakeCfg.Cache.InventoryValidity = defaultInventoryValiditySeconds
	fakeCfg.InventoryPrefix = "sat_"
	fakeCfg.WriteConfig(testFile.Name())

	cfg, err := ParseConfig(testFile.Name())
	if err != nil {
		t.Fatalf("ParseConfig returned: %v", err)
	}

	if cfg.ValidDays != fakeCfg.ValidDays || cfg.ValidDays != defaultSatValidDays {
		t.Fatalf(
			"Unexpected config.ValidDays. Default=%d, Expected=%d, Got=%d",
			defaultSatValidDays, fakeCfg.ValidDays, cfg.ValidDays)
	}
	if !cfg.SatValidExclude("svexclude") {
		t.Error("SatValidExclude does not include the string svexclude")
	}
	if cfg.Cache.Validity != fakeCfg.Cache.Validity || cfg.Cache.Validity != defaultCacheValiditySeconds {
		t.Fatalf(
			"Unexpected config.Cache.Validity. Default=%d, Expected=%d, Got=%d",
			defaultCacheValiditySeconds, fakeCfg.Cache.Validity, cfg.Cache.Validity)
	}
	if cfg.Cache.InventoryValidity != fakeCfg.Cache.InventoryValidity || cfg.Cache.InventoryValidity != defaultInventoryValiditySeconds {
		t.Fatalf(
			"Unexpected config.Cache.InventoryValidity. Default=%d, Expected=%d, Got=%d",
			defaultInventoryValiditySeconds, fakeCfg.Cache.InventoryValidity, cfg.Cache.InventoryValidity)
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
