package cmd

import (
	"slices"
	"testing"

	"github.com/shyim/agm/internal/aliases"
	"github.com/shyim/agm/internal/db"
)

func TestCompleteAccountsAndAliases(t *testing.T) {
	// Isolate the test run by using a temporary directory for data
	tmpDir := t.TempDir()
	t.Setenv("AGM_DATA_DIR", tmpDir)

	// Ensure the store is initialized in the temp directory
	if err := db.EnsureStore(); err != nil {
		t.Fatalf("EnsureStore failed: %v", err)
	}

	// Insert test accounts
	tok := &db.Token{AccessToken: "access", RefreshToken: "refresh"}
	err := db.UpsertAccount("test1@example.com", "Test One", "", tok, nil)
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}
	err = db.UpsertAccount("test2@example.com", "Test Two", "", tok, nil)
	if err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}

	// Set an alias
	err = aliases.Set("t1", "test1@example.com")
	if err != nil {
		t.Fatalf("Set alias failed: %v", err)
	}

	// Retrieve completions
	completions, directive := completeAccountsAndAliases(nil, nil, "")
	if directive != 4 { // cobra.ShellCompDirectiveNoFileComp is 4
		t.Errorf("Expected directive 4, got %v", directive)
	}

	expected := []string{
		"test1@example.com\tTest One",
		"test2@example.com\tTest Two",
		"t1\tAlias for test1@example.com",
	}

	for _, exp := range expected {
		if !slices.Contains(completions, exp) {
			t.Errorf("Expected completion %q not found in %v", exp, completions)
		}
	}
}
