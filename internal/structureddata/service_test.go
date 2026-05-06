package structureddata

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizeOwnerAndGeneratedNames(t *testing.T) {
	owner, ownerID, err := normalizeOwner(OwnerRef{
		Type:  OwnerTypeBot,
		BotID: "123e4567-e89b-12d3-a456-426614174000",
	})
	if err != nil {
		t.Fatalf("normalizeOwner returned error: %v", err)
	}
	if owner.Type != OwnerTypeBot {
		t.Fatalf("owner type = %q, want %q", owner.Type, OwnerTypeBot)
	}
	if ownerID != "123e4567e89b12d3a456426614174000" {
		t.Fatalf("owner id = %q", ownerID)
	}
	schemaName, roleName := generatedNames(owner.Type, ownerID)
	if schemaName != "bot_data_123e4567e89b12d3a456426614174000" {
		t.Fatalf("schema name = %q", schemaName)
	}
	if roleName != "memoh_bot_123e4567e89b12d3a456426614174000" {
		t.Fatalf("role name = %q", roleName)
	}
	if err := validateGeneratedIdentifier(schemaName); err != nil {
		t.Fatalf("schema identifier invalid: %v", err)
	}
	if err := validateGeneratedIdentifier(roleName); err != nil {
		t.Fatalf("role identifier invalid: %v", err)
	}
}

func TestNormalizeOwnerRejectsMixedOwner(t *testing.T) {
	_, _, err := normalizeOwner(OwnerRef{
		Type:       OwnerTypeBot,
		BotID:      "123e4567-e89b-12d3-a456-426614174000",
		BotGroupID: "123e4567-e89b-12d3-a456-426614174001",
	})
	if !errors.Is(err, ErrInvalidOwner) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidOwner)
	}
}

func TestNormalizePrivileges(t *testing.T) {
	got, err := normalizePrivileges([]string{"write", "read", "write", "ddl"})
	if err != nil {
		t.Fatalf("normalizePrivileges returned error: %v", err)
	}
	want := []string{"ddl", "read", "write"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("privileges = %#v, want %#v", got, want)
	}
}

func TestNormalizePrivilegesRejectsEmptyOrUnknown(t *testing.T) {
	if _, err := normalizePrivileges(nil); !errors.Is(err, ErrInvalidPrivilege) {
		t.Fatalf("empty error = %v, want %v", err, ErrInvalidPrivilege)
	}
	if _, err := normalizePrivileges([]string{"admin"}); !errors.Is(err, ErrInvalidPrivilege) {
		t.Fatalf("unknown error = %v, want %v", err, ErrInvalidPrivilege)
	}
}

func TestNormalizeMaxRows(t *testing.T) {
	cases := []struct {
		input int32
		want  int32
	}{
		{input: 0, want: defaultMaxRows},
		{input: 1, want: 1},
		{input: 9000, want: absoluteMaxRows},
	}
	for _, tc := range cases {
		if got := normalizeMaxRows(tc.input); got != tc.want {
			t.Fatalf("normalizeMaxRows(%d) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
