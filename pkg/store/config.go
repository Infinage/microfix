package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
	"strconv"
)

// Reflect and store the config fields
// This will be used for auto completion
var configFields []string

type Config struct {
	SenderCompID string `json:"SenderCompID"`
	TargetCompID string `json:"TargetCompID"`
	HeartbeatInt int64  `json:"HeartbeatInt"`

	SpecPath             string `json:"SpecPath"`
	DefaultApplVer       string `json:"DefaultApplVer"`
	SpecDisplayOptFields bool   `json:"SpecDisplayOptFields"`

	SkipLatencyCheckInValidate bool `json:"SkipLatencyCheckInValidate"`

	FixValidateStrict bool `json:"FixValidateStrict"`
	FixSampleOptional bool `json:"FixSampleOptional"`

	IpAddr string `json:"IpAddr"`
	Port   uint16 `json:"Port"`

	Alias map[string]string `json:"Alias"`

	DefaultTimeoutSec uint32 `json:"DefaultTimeoutSec"`
}

func (cfg *Config) dump(filepath string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, data, 0644)
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

// Tries to load .mxrc file from CWD & Home directory
// If not found returns struct with default configs
// Returns path at which it was initialized
func initConfig() (Config, string) {
	// Attempt to load from './.mxrc' first
	cwd, _ := os.Getwd()
	filepath_cwd := path.Join(cwd, ".mxrc")
	cfg, err := loadConfig(filepath_cwd)
	if err == nil {
		return *cfg, filepath_cwd
	}

	// Attempt to load from '~/.mxrc'
	homeDir, _ := os.UserHomeDir()
	filepath_homedir := path.Join(homeDir, ".mxrc")
	cfg, err = loadConfig(filepath_homedir)
	if err == nil {
		return *cfg, filepath_homedir
	}

	// Failing both, creating a new config and dump to './.mxrc'
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
		Alias:                make(map[string]string),
		DefaultTimeoutSec:    5,
	}

	return *cfg, filepath_cwd
}

// Returns the alias value and a boolean indicating if it exists
func (cfg *Config) getAlias(key string) (string, bool) {
	if cfg.Alias == nil {
		return "", false
	}
	val, ok := cfg.Alias[key]
	return val, ok
}

// Sets or updates an alias.
// Returns the old value (if any) and true if it was an update, false if it was a new insert.
func (cfg *Config) setAlias(key, value string) (string, bool) {
	if cfg.Alias == nil {
		cfg.Alias = make(map[string]string)
	}

	oldVal, ok := cfg.Alias[key]
	cfg.Alias[key] = value
	return oldVal, ok
}

// Removes an alias. Returns the deleted value and true if it existed.
func (cfg *Config) deleteAlias(key string) (string, bool) {
	if cfg.Alias == nil {
		return "", false
	}

	oldVal, ok := cfg.Alias[key]
	delete(cfg.Alias, key)
	return oldVal, ok
}

// Uses reflection to safely retrieve a strict config field as a string
func (cfg *Config) getField(key string) (string, error) {
	v := reflect.ValueOf(cfg).Elem()
	field := v.FieldByName(key)

	// Disallow access to Alias, user should use GetAlias instead
	if !field.IsValid() || key == "Alias" {
		return "", fmt.Errorf("Config field '%s' not found", key)
	}

	// Print underlying type's string representation
	return fmt.Sprint(field.Interface()), nil
}

// Updates a struct field from a string value.
// Returns previous value as a string
func (cfg *Config) setField(key, value string) (string, error) {
	v := reflect.ValueOf(cfg).Elem()
	field := v.FieldByName(key)

	// Disallow access to Alias, user should use GetAlias instead
	if !field.IsValid() || !field.CanSet() || key == "Alias" {
		return "", fmt.Errorf("Config field '%s' not found or cannot be modified", key)
	}

	oldVal := fmt.Sprint(field.Interface())

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int64:
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			field.SetInt(i)
		} else {
			return oldVal, fmt.Errorf("Invalid integer '%s'", value)
		}

	case reflect.Uint16, reflect.Uint32:
		if u, err := strconv.ParseUint(value, 10, 64); err == nil {
			field.SetUint(u)
		} else {
			return oldVal, fmt.Errorf("Invalid unsigned integer '%s'", value)
		}

	case reflect.Bool:
		if b, err := strconv.ParseBool(value); err == nil {
			field.SetBool(b)
		} else {
			return oldVal, fmt.Errorf("Invalid boolean '%s'", value)
		}

	default:
		return oldVal, fmt.Errorf("Unsupported type %s", field.Kind())
	}

	return oldVal, nil
}

// Returns a copy of the reflected field struct
func ConfigFields() []string {
	return append([]string{}, configFields...)
}

func init() {
	for _, field := range reflect.VisibleFields(reflect.TypeFor[Config]()) {
		configFields = append(configFields, field.Name)
	}
}
