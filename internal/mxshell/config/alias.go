package config

import (
	"encoding/json"
	"os"
	"path"
)

type Alias map[string]string

func (alias *Alias) Dump(filepath string) error {
	data, err := json.MarshalIndent(alias, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0644)
}

func LoadAlias(filepath string) (*Alias, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var alias = new(Alias)
	err = json.NewDecoder(file).Decode(alias)

	return alias, err
}

// Tries to load .mxalias file from CWD & Home directory
// If not found returns struct with default configs
func InitAlias() Alias {
	// Load from current working directory
	if alias, err := LoadAlias(".mxalias"); err == nil {
		return *alias
	}

	// Load from home directory
	homeDir, _ := os.UserHomeDir()
	if alias, err := LoadAlias(path.Join(homeDir, ".mxalias")); err == nil {
		return *alias
	}

	// Create a new one
	return make(Alias)
}
