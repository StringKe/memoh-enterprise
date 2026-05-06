package security

import "testing"

func TestLimits(t *testing.T) {
	t.Parallel()

	if RawChunkSize != 1_048_576 {
		t.Fatalf("RawChunkSize = %d, want 1048576", RawChunkSize)
	}
	if UnaryReadMaxBytes != 16_777_216 {
		t.Fatalf("UnaryReadMaxBytes = %d, want 16777216", UnaryReadMaxBytes)
	}
	if ListPageSize != 200 {
		t.Fatalf("ListPageSize = %d, want 200", ListPageSize)
	}
}
