package config

import (
	"flag"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

// Config contains all the configuration settings for Yamn.
type Config struct {
	APIBaseURL      string `yaml:"api_baseurl"`
	APICertFile     string `yaml:"api_certfile"`
	APIPassword     string `yaml:"api_password"`
	APIUser         string `yaml:"api_user"`
	CacheDIR	string `yaml:"cache_dir"`
	CacheValidity int64 `yaml:"cache_validity"`
	InventoryPrefix string `yaml:"inventory_prefix"`
	OutJSON         string `yaml:"target_filename"`
	SamplePath      string `yaml:"sample_path"`
}

// Flags are the command line flags
type Flags struct {
	Config string
	Debug bool
	List bool
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
	flag.StringVar(&f.Config, "config", "/etc/ansible/satinv.yml", "Config file")
	flag.BoolVar(&f.Debug, "debug", false, "Write logoutput to stderr")
	flag.BoolVar(&f.List, "list", false, "Produce a full inventory to stdout")
	flag.Parse()

	// If a "--config" flag hasn't been provided, try reading a YAMNCFG environment variable.
	if f.Config == "" && os.Getenv("SATINVCFG") != "" {
		f.Config = os.Getenv("SATINVCFG")
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
	return config, nil
}
