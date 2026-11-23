package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// EnvPrefix starts every environment override.
const EnvPrefix = "SKYGRID_"

// EnvKey returns the environment variable that overrides section.key, e.g.
// EnvKey("coordinator", "lock_timeout_ms") == "SKYGRID_COORDINATOR_LOCK_TIMEOUT_MS".
func EnvKey(section, key string) string {
	return EnvPrefix + strings.ToUpper(section) + "_" + strings.ToUpper(key)
}

// ShardEnvKey returns the environment variable that overrides a shard's DSN,
// e.g. ShardEnvKey(2) == "SKYGRID_SHARD_2_DSN".
func ShardEnvKey(id int) string {
	return EnvPrefix + "SHARD_" + strconv.Itoa(id) + "_DSN"
}

// applyEnv overrides scalar fields of every section from the environment.
// Field names come from the yaml tags, so new config keys are picked up
// automatically.
func applyEnv(cfg *Config, env LookupEnv) error {
	root := reflect.ValueOf(cfg).Elem()
	rootType := root.Type()
	for i := range rootType.NumField() {
		section := root.Field(i)
		if section.Kind() != reflect.Struct {
			continue // shards are handled below
		}
		sectionName := yamlName(rootType.Field(i))
		sectionType := section.Type()
		for j := range sectionType.NumField() {
			key := EnvKey(sectionName, yamlName(sectionType.Field(j)))
			raw, ok := env(key)
			if !ok {
				continue
			}
			if err := setScalar(section.Field(j), raw); err != nil {
				return fmt.Errorf("env %s: %w", key, err)
			}
		}
	}
	for i := range cfg.Shards {
		if dsn, ok := env(ShardEnvKey(cfg.Shards[i].ID)); ok {
			cfg.Shards[i].DSN = dsn
		}
	}
	return nil
}

func yamlName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
	if name == "" {
		return strings.ToLower(f.Name)
	}
	return name
}

func setScalar(v reflect.Value, raw string) error {
	raw = strings.TrimSpace(raw)
	switch v.Kind() {
	case reflect.String:
		v.SetString(raw)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("want an integer, got %q", raw)
		}
		v.SetInt(n)
	case reflect.Float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("want a number, got %q", raw)
		}
		v.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("want true or false, got %q", raw)
		}
		v.SetBool(b)
	default:
		return fmt.Errorf("unsupported field kind %s", v.Kind())
	}
	return nil
}
