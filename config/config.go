package config

import (
	"flag"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

// Config contains all the configuration settings for Yamn.
type Config struct {
	OutJSON         string `yaml:"target_filename"`
	InventoryPrefix string `yaml:"inventory_prefix"`
}

// Flags are the command line flags
type Flags struct {
	Config string
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
