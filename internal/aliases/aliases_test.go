package aliases

import (
	"testing"
)

func TestAliasesInvertedAndResolve(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("AGM_DATA_DIR", tmpDir)

	// 1. Normal set: name -> email
	if err := Set("work", "work@example.com"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if got := Resolve("work"); got != "work@example.com" {
		t.Fatalf("expected work@example.com, got %s", got)
	}

	// 2. Inverted set: email -> name (e.g. user typed 'agm alias email alias')
	// Should auto-swap to name -> email and remove stale reverse mapping
	if err := Set("user1@example.com", "user1"); err != nil {
		t.Fatalf("Set inverted failed: %v", err)
	}
	m := Load()
	if m["user1"] != "user1@example.com" {
		t.Fatalf("expected user1 -> user1@example.com, got %v", m)
	}
	if _, exists := m["user1@example.com"]; exists {
		t.Fatalf("stale reverse key user1@example.com should not exist")
	}

	// 3. Resolve alias name should return email
	if got := Resolve("user1"); got != "user1@example.com" {
		t.Fatalf("expected user1@example.com, got %s", got)
	}

	// 4. Resolve original email should return email itself (not corrupted)
	if got := Resolve("user1@example.com"); got != "user1@example.com" {
		t.Fatalf("expected user1@example.com, got %s", got)
	}
}
