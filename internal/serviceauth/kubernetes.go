package serviceauth

import (
	"errors"
	"strings"
)

type KubernetesServiceAccountIdentity struct {
	Audience           string
	Namespace          string
	ServiceAccountName string
}

type KubernetesServiceAccountValidator struct {
	audience           string
	namespace          string
	serviceAccountName string
}

func NewKubernetesServiceAccountValidator(audience, namespace, serviceAccountName string) *KubernetesServiceAccountValidator {
	return &KubernetesServiceAccountValidator{
		audience:           strings.TrimSpace(audience),
		namespace:          strings.TrimSpace(namespace),
		serviceAccountName: strings.TrimSpace(serviceAccountName),
	}
}

func (v *KubernetesServiceAccountValidator) Validate(identity KubernetesServiceAccountIdentity) error {
	if v == nil {
		return errors.New("kubernetes service account validator is not configured")
	}
	if strings.TrimSpace(v.audience) == "" || strings.TrimSpace(v.namespace) == "" || strings.TrimSpace(v.serviceAccountName) == "" {
		return errors.New("kubernetes service account validator is incomplete")
	}
	if strings.TrimSpace(identity.Audience) != v.audience {
		return ErrPermissionDenied
	}
	if strings.TrimSpace(identity.Namespace) != v.namespace {
		return ErrPermissionDenied
	}
	if strings.TrimSpace(identity.ServiceAccountName) != v.serviceAccountName {
		return ErrPermissionDenied
	}
	return nil
}
