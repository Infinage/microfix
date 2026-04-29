package main

import (
	"encoding/json"
	"os"
	"path"
)

type Config struct {
	SenderCompID string `json:"SenderCompID"`
	TargetCompID string `json:"TargetCompID"`
	HeartbeatInt int64  `json:"HeartbeatInt"`

	SpecPath             string `json:"SpecPath"`
	SpecDisplayOptFields bool   `json:"SpecDisplayOptFields"`

	IpAddr string `json:"IpAddr"`
	Port   uint16 `json:"Port"`
}

// Attempt to load and unmarshal into config, returns err on failure
func loadConfig(filepath string) (*Config, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	var config = new(Config)
	err = json.NewDecoder(file).Decode(config)

	return config, err
}

func dumpConfig(filepath string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0644)
}

// Tries to load .mxrc file from CWD & Home directory
// If not found returns struct with default configs
func InitConfig() Config {
	cfg, err := loadConfig(".mxrc")
	if err == nil {
		return *cfg
	}

	homeDir, _ := os.UserHomeDir()
	cfg, err = loadConfig(path.Join(homeDir, ".mxrc"))
	if err == nil {
		return *cfg
	}

	cfg = &Config{
		SenderCompID:         "SENDER",
		TargetCompID:         "TARGET",
		HeartbeatInt:         30,
		SpecPath:             "FIX44.xml",
		SpecDisplayOptFields: false,
		IpAddr:               "0.0.0.0",
		Port:                 1234,
	}

	// Write default template to currency working directory
	dumpConfig(".mxrc", cfg)

	return *cfg
}
