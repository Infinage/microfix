package config

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
	DefaultApplVer       string `json:"DefaultApplVer"`
	SpecDisplayOptFields bool   `json:"SpecDisplayOptFields"`

	FixValidateStrict bool `json:"FixValidateStrict"`
	FixSampleOptional bool `json:"FixSampleOptional"`

	IpAddr string `json:"IpAddr"`
	Port   uint16 `json:"Port"`
}

func (cfg *Config) Dump(filepath string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0644)
}

// Attempt to load and unmarshal into config, returns err on failure
func LoadConfig(filepath string) (*Config, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}

	defer file.Close()

	var config = new(Config)
	err = json.NewDecoder(file).Decode(config)

	return config, err
}

// Tries to load .mxrc file from CWD & Home directory
// If not found returns struct with default configs
func InitConfig() Config {
	cfg, err := LoadConfig(".mxrc")
	if err == nil {
		return *cfg
	}

	homeDir, _ := os.UserHomeDir()
	cfg, err = LoadConfig(path.Join(homeDir, ".mxrc"))
	if err == nil {
		return *cfg
	}

	cfg = &Config{
		SenderCompID:         "SENDER",
		TargetCompID:         "TARGET",
		HeartbeatInt:         30,
		SpecPath:             "FIX44.xml",
		SpecDisplayOptFields: false,
		FixValidateStrict:    true,
		FixSampleOptional:    false,
		IpAddr:               "0.0.0.0",
		Port:                 1234,
	}

	// Write default template to currency working directory
	cfg.Dump(".mxrc")

	return *cfg
}
