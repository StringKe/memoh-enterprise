package main

import "testing"

func TestRootCommandIsOperationsOnly(t *testing.T) {
	root := newRootCommand()
	forbidden := map[string]bool{
		"bots":  true,
		"bot":   true,
		"chat":  true,
		"login": true,
	}
	required := map[string]bool{
		"migrate": true,
		"install": true,
		"serve":   true,
		"ctr":     true,
		"start":   true,
		"stop":    true,
		"restart": true,
		"status":  true,
		"logs":    true,
		"update":  true,
		"version": true,
		"admin":   true,
		"support": true,
	}
	for _, cmd := range root.Commands() {
		if forbidden[cmd.Name()] {
			t.Fatalf("business command %q must not be registered", cmd.Name())
		}
		delete(required, cmd.Name())
	}
	if len(required) > 0 {
		t.Fatalf("missing operations commands: %#v", required)
	}
}
