package serviceauth

import (
	"errors"
	"testing"
)

func TestKubernetesServiceAccountValidator(t *testing.T) {
	validator := NewKubernetesServiceAccountValidator("memoh-internal", "memoh", "memoh-agent-runner")
	valid := KubernetesServiceAccountIdentity{
		Audience:           "memoh-internal",
		Namespace:          "memoh",
		ServiceAccountName: "memoh-agent-runner",
	}
	if err := validator.Validate(valid); err != nil {
		t.Fatal(err)
	}
	for name, input := range map[string]KubernetesServiceAccountIdentity{
		"wrong namespace": {
			Audience:           "memoh-internal",
			Namespace:          "other",
			ServiceAccountName: "memoh-agent-runner",
		},
		"wrong service account": {
			Audience:           "memoh-internal",
			Namespace:          "memoh",
			ServiceAccountName: "memoh-connector",
		},
		"wrong audience": {
			Audience:           "other",
			Namespace:          "memoh",
			ServiceAccountName: "memoh-agent-runner",
		},
	} {
		if err := validator.Validate(input); !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("%s error = %v", name, err)
		}
	}
}
