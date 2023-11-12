// config provides flag and configuration file reading for satinv
package config

import (
	"flag"
	"os"
	"os/user"
	"path"
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	defaultSatValidHours            int   = 48
	defaultCacheValiditySeconds     int64 = 8 * 60 * 60 // 8 Hours
	defaultInventoryValiditySeconds int64 = 2 * 60 * 60 // 2 Hours
)

// Config contains all the configuration settings
type Config struct {
	API struct {
		BaseURL  string `yaml:"baseurl"`
		CertFile string `yaml:"certfile"`
		Password string `yaml:"password"`
		User     string `yaml:"user"`
	} `yaml:"api"`
	Cache struct {
		Dir                 string `yaml:"dir"`
		ValidityHosts       int64  `yaml:"validity_hosts"`
		ValidityCollections int64  `yaml:"validity_collections"`
		ValidityInventory   int64  `yaml:"validity_inventory"`
	} `yaml:"cache"`
	CIDRs           map[string]string `yaml:"cidrs"`
	InventoryPrefix string            `yaml:"inventory_prefix"`
	Logging         struct {
		Journal  bool   `yaml:"journal"`
		LevelStr string `yaml:"level"`
		Filename string `yaml:"filename"`
	} `yaml:"logging"`
	Valid struct {
		Hours        int      `yaml:"hours"`
		Unlicensed   bool     `yaml:"include_unlicensed"`
		ExcludeHosts []string `yaml:"exclude_hosts"`
		ExcludeRegex []string `yaml:"exclude_regex"`
	} `yaml:"valid"`
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
	err = os.WriteFile(filename, data, 0644)
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

	// If a "--config" flag has been provided, it should be honoured (even if it's invalid or doesn't exist).
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
	// Read the config file
	if err := y.Decode(&config); err != nil {
		return nil, err
	}
	// Set config defaults here
	if config.Valid.Hours == 0 {
		config.Valid.Hours = defaultSatValidHours
	}
	if config.Cache.ValidityHosts == 0 {
		config.Cache.ValidityHosts = defaultCacheValiditySeconds
	}
	if config.Cache.ValidityCollections == 0 {
		config.Cache.ValidityCollections = defaultCacheValiditySeconds
	}
	if config.Cache.ValidityInventory == 0 {
		config.Cache.ValidityInventory = defaultInventoryValiditySeconds
	}

	// The following config options may need tilde expansion
	config.Cache.Dir = expandTilde(config.Cache.Dir)
	config.Logging.Filename = expandTilde(config.Logging.Filename)

	return config, nil
}

// expandTilde expands filenames and paths that use the tilde convention to imply relative to homedir.
func expandTilde(inPath string) (outPath string) {
	u, err := user.Current()
	if err != nil {
		panic(err)
	}
	if inPath == "~" {
		outPath = u.HomeDir
	} else if strings.HasPrefix(inPath, "~/") {
		outPath = path.Join(u.HomeDir, inPath[2:])
	} else {
		outPath = inPath
	}
	return
}
