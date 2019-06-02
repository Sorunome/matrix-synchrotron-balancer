package config

import (
	"log"
	"io/ioutil"
	"gopkg.in/yaml.v2"
)

type SynchrotronConfig struct {
	Url string `yaml:"url"`
	PIDFile string `yaml:"pid_file"`
}

type BalancerConfig struct {
	RelocateThreashold float64 `yaml:"relocate_threashold"`
	RelocateCounterThreashold int `yaml:"relocate_counter_threashold"`
	Interval int `yaml:"interval"`
}

type Config struct {
	HomeserverUrl string `yaml:"homeserver_url"`
	Listener string `yaml:"listener"`
	Synchrotrons []*SynchrotronConfig `yaml:"synchrotrons"`
	Balancer *BalancerConfig `yaml:"balancer"`
}

var instance *Config = nil
var Path = "config.yaml"

func defaultConfig() *Config {
	return &Config{
		HomeserverUrl: "http://localhost:8008",
		Listener: "localhost:8083",
		Synchrotrons: []*SynchrotronConfig{
			&SynchrotronConfig{
				Url: "localhost:8008",
				PIDFile: "/tmp/pid",
			},
		},
		Balancer: &BalancerConfig{
			RelocateThreashold: 3.0,
			RelocateCounterThreashold: 5,
			Interval: 60,
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
