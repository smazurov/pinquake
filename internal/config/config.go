package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"


	"github.com/pelletier/go-toml/v2"
)

// LoadConfig loads configuration with precedence: CLI flags > env vars > config file > defaults.
func LoadConfig(opts any) error {
	v := reflect.ValueOf(opts).Elem()
	t := v.Type()

	var configPath string
	for i := 0; i < v.NumField(); i++ {
		if t.Field(i).Name == "Config" {
			configPath = v.Field(i).String()
			break
		}
	}

	if configPath != "" {
		if data, err := os.ReadFile(configPath); err == nil {
			var config map[string]any
			if err := toml.Unmarshal(data, &config); err != nil {
				return fmt.Errorf("failed to parse TOML config: %w", err)
			}
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				fieldType := t.Field(i)
				if tomlPath := fieldType.Tag.Get("toml"); tomlPath != "" {
					if value := getNestedValue(config, tomlPath); value != nil {
						setFieldValue(field, value)
					}
				}
			}
		}
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		if envKey := fieldType.Tag.Get("env"); envKey != "" {
			if envValue := os.Getenv("PINQUAKE_" + envKey); envValue != "" {
				setFieldValueFromString(field, envValue)
			}
		}
	}

	return nil
}

// ApplyDefaults sets fields to their default tag values if they are zero.
func ApplyDefaults(opts any) {
	v := reflect.ValueOf(opts).Elem()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		if field.IsZero() {
			if def := fieldType.Tag.Get("default"); def != "" {
				setFieldValueFromString(field, def)
			}
		}
	}
}

func getNestedValue(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			return current[part]
		}
		if next, ok := current[part].(map[string]any); ok {
			current = next
		} else {
			return nil
		}
	}
	return nil
}

func setFieldValue(field reflect.Value, value any) {
	if !field.CanSet() {
		return
	}
	switch field.Kind() {
	case reflect.String:
		if s, ok := value.(string); ok {
			field.SetString(s)
		}
	case reflect.Bool:
		if b, ok := value.(bool); ok {
			field.SetBool(b)
		}
	case reflect.Int:
		if i, ok := value.(int64); ok {
			field.SetInt(i)
		}
	}
}

func setFieldValueFromString(field reflect.Value, value string) {
	if !field.CanSet() {
		return
	}
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Bool:
		if b, err := strconv.ParseBool(value); err == nil {
			field.SetBool(b)
		}
	case reflect.Int:
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			field.SetInt(i)
		}
	}
}
