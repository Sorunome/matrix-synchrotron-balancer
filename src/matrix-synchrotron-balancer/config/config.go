package config

import (
	"log"
	"os"
	"io/ioutil"
	"encoding/json"
)

type SynchrotronConfig struct {
	Url string `json:"url"`
	PIDFile string `json:"pid_file"`
}

type Config struct {
	HomeserverUrl string `json:"homeserver_url"`
	Listener string `json:"listener"`
	Synchrotrons []*SynchrotronConfig `json:"synchrotrons"`
}

var instance *Config = nil
var Path = "config.json"

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
	}
}

func loadConfig() error {
	c := defaultConfig()

	f, err := os.Open(Path)
	if err != nil {
		return err
	}
	defer f.Close()

	buffer, err := ioutil.ReadAll(f)
	if err != nil {
		return err
	}
	err = json.Unmarshal(buffer, &c)
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
