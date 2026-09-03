package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewVersionCheckerUsesApplicationDataDirectory(t *testing.T) {
	oldWorkingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	oldUseHomeData := useHomeData
	useHomeData = false

	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		useHomeData = oldUseHomeData
		if err := os.Chdir(oldWorkingDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	checker := NewVersionChecker("1.0.0", "code-kanban")
	want := filepath.Join(tempDir, "data", "version-cache.json")
	got, err := filepath.Abs(checker.cacheFile)
	if err != nil {
		t.Fatalf("resolve cache path: %v", err)
	}
	if got != want {
		t.Fatalf("cache file = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Dir(got)); err != nil {
		t.Fatalf("stat data directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("cache parent %q is not a directory", filepath.Dir(got))
	}
}
