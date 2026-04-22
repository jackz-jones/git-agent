package storage

import (
	"os"
	"testing"
)

func TestNew(t *testing.T) {
	s, err := New("/tmp/test-git-agent-storage")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if s == nil {
		t.Fatal("Storage should not be nil")
	}
	defer os.RemoveAll("/tmp/test-git-agent-storage")
}
