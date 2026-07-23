package checks

import "testing"

func TestFilterRemovesIgnoredChecks(t *testing.T) {
	filtered, err := Filter(All(), []string{"pdb", "local-storage"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, check := range filtered {
		if check.Name() == "pdb" || check.Name() == "local-storage" {
			t.Errorf("check %q should have been filtered out", check.Name())
		}
	}
	if len(filtered) != len(All())-2 {
		t.Errorf("expected %d checks, got %d", len(All())-2, len(filtered))
	}
}

func TestFilterRejectsUnknownCheck(t *testing.T) {
	if _, err := Filter(All(), []string{"no-such-check"}); err == nil {
		t.Fatal("expected an error for an unknown check name")
	}
}
