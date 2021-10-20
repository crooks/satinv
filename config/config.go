// config provides flag and configuration file reading for satinv
package config

import (
	"flag"
	"io/ioutil"
	"os"
	"os/user"
	"path"

	"gopkg.in/yaml.v2"
)

// Config contains all the configuration settings for Yamn.
type Config struct {
	APIBaseURL      string            `yaml:"api_baseurl"`
	APICertFile     string            `yaml:"api_certfile"`
	APIPassword     string            `yaml:"api_password"`
	APIUser         string            `yaml:"api_user"`
	CacheHomeDir    bool              `yaml:"cache_homedir"`
	CacheDir        string            `yaml:"cache_dir"`
	CacheValidity   int64             `yaml:"cache_validity"`
	CIDRs           map[string]string `yaml:"cidrs"`
	InventoryPrefix string            `yaml:"inventory_prefix"`
	OutJSON         string            `yaml:"target_filename"`
	SatValidDays    int               `yaml:"sat_valid_days"`
}

// Flags are the command line flags
type Flags struct {
	Config  string
	Debug   bool
	List    bool
	Refresh bool
}

// WriteConfig will create a YAML formatted config file from a Config struct
func (c *Config) WriteConfig(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

// ParseFlags transcribes command line flags into a struct
func ParseFlags() *Flags {
	f := new(Flags)
	// Config file
	flag.StringVar(&f.Config, "config", "", "Config file")
	flag.BoolVar(&f.Debug, "debug", false, "Write logoutput to stderr")
	flag.BoolVar(&f.List, "list", false, "Produce a full inventory to stdout")
	flag.BoolVar(&f.Refresh, "refresh", false, "Force a cache refresh")
	flag.Parse()

	// If a "--config" flag has been provided, it should be honoured (even if it's invalid of doesn't exist).
	if f.Config == "" {
		if os.Getenv("SATINVCFG") == "" {
			// Environment variable hasn't been set.  No options left so take a bold guess at a config location.
			f.Config = "/etc/ansible/satinv.yml"
		} else {
			// Assume the SATINVCFG variable contains something meaningful and valid.
			f.Config = os.Getenv("SATINVCFG")
		}
	}
	return f
}

// ParseConfig expects a YAML formatted config file and populates a Config struct
func ParseConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	y := yaml.NewDecoder(file)
	config := new(Config)
	if err := y.Decode(&config); err != nil {
		return nil, err
	}
	// Default sat_valid period to one week
	if config.SatValidDays == 0 {
		config.SatValidDays = 7
	}
	// Set default cache validity to 8 hours
	if config.CacheValidity == 0 {
		config.CacheValidity = 8 * 60 * 60
	}
	// Set default inventory InventoryPrefix
	if config.InventoryPrefix == "" {
		config.InventoryPrefix = "sat_"
	}
	// Set default target_filename
	if config.OutJSON == "" {
		config.OutJSON = "/tmp/satinv.json"
	}
	// If the CacheHomeDir boolean is true, config.CacheDir will be relative to the user's HomeDir.
	// This overcomes issues with multiple users trying to write to a common cache.
	if config.CacheHomeDir {
		user, err := user.Current()
		if err != nil {
			panic(err)
		}
		config.CacheDir = path.Join(user.HomeDir, config.CacheDir)
	}
	return config, nil
}
