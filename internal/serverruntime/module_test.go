package serverruntime

import (
	"testing"

	"go.uber.org/fx"
)

func TestOptionsValidateDependencyGraph(t *testing.T) {
	t.Parallel()

	if err := fx.ValidateApp(options()); err != nil {
		t.Fatal(err)
	}
}
