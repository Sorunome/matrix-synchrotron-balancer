package config

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v2"
)

type SynchrotronConfig struct {
	Address string `yaml:"address"`
	PIDFile string `yaml:"pid_file"`
}

type BalancerConfig struct {
	RelocateThreshold        float64 `yaml:"relocate_threshold"`
	RelocateCounterThreshold float64 `yaml:"relocate_counter_threshold"`
	RelocateMinCpu           float64 `yaml:"relocate_min_cpu"`
	RelocateCooldown         float64 `yaml:"relocate_cooldown"`
	Interval                 int     `yaml:"interval"`
}

type Config struct {
	HomeserverUrl string               `yaml:"homeserver_url"`
	Listener      string               `yaml:"listener"`
	Synchrotrons  []*SynchrotronConfig `yaml:"synchrotrons"`
	Balancer      *BalancerConfig      `yaml:"balancer"`
}

var instance *Config = nil
var Path = "config.yaml"

func defaultConfig() *Config {
	return &Config{
		HomeserverUrl: "http://localhost:8008",
		Listener:      "localhost:8083",
		Synchrotrons:  nil,
		Balancer: &BalancerConfig{
			RelocateThreshold:        3.0,
			RelocateCounterThreshold: 4.5,
			RelocateCooldown:         0.2,
			Interval:                 2,
			RelocateMinCpu:           10.0,
		},
	}
}

func loadConfig() error {
	c := defaultConfig()

	buffer, err := ioutil.ReadFile(Path)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(buffer, &c)
	if err != nil {
		return err
	}
	instance = c
	return nil
}

func Get() *Config {
	if instance != nil {
		return instance
	}
	err := loadConfig()
	if err != nil {
		log.Panic("Error loading config: ", err)
	}
	return instance
}
