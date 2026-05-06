package security

import (
	"errors"
	"sort"
	"strings"
)

var ErrEnvForbidden = errors.New("environment variable is not allowed")

var allowedEnvKeys = map[string]struct{}{
	"HOME":               {},
	"PATH":               {},
	"PWD":                {},
	"SHELL":              {},
	"TERM":               {},
	"TMPDIR":             {},
	"LANG":               {},
	"LC_ALL":             {},
	"LC_CTYPE":           {},
	"USER":               {},
	"USERNAME":           {},
	"HOSTNAME":           {},
	"MEMOH_WORKSPACE_ID": {},
	"MEMOH_RUN_ID":       {},
	"MEMOH_BOT_ID":       {},
	"MEMOH_SESSION_ID":   {},
}

func EnvAllowed(key string) bool {
	_, ok := allowedEnvKeys[strings.TrimSpace(key)]
	return ok
}

func AllowedEnvKeys() []string {
	keys := make([]string, 0, len(allowedEnvKeys))
	for key := range allowedEnvKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func SanitizeEnv(defaults, requested []string) ([]string, error) {
	values := make(map[string]string, len(defaults)+len(requested))
	for _, item := range defaults {
		key, value, ok := splitEnv(item)
		if !ok || !EnvAllowed(key) {
			continue
		}
		values[key] = value
	}
	for _, item := range requested {
		key, value, ok := splitEnv(item)
		if !ok || !EnvAllowed(key) {
			return nil, ErrEnvForbidden
		}
		values[key] = value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out, nil
}

func splitEnv(item string) (string, string, bool) {
	key, value, ok := strings.Cut(item, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" || strings.ContainsAny(key, "\x00=") || strings.Contains(value, "\x00") {
		return "", "", false
	}
	return key, value, true
}
