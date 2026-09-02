package websession

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestPiBridgeMaterializerConcurrentInstall(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	const workers = 32
	paths := make([]string, workers)
	errors := make([]error, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := range workers {
		go func() {
			defer wait.Done()
			paths[index], errors[index] = manager.materializePiBridge()
		}()
	}
	wait.Wait()

	for index := range workers {
		if errors[index] != nil {
			t.Fatalf("materialize %d: %v", index, errors[index])
		}
		if paths[index] != paths[0] {
			t.Fatalf("materialize %d path = %q, want %q", index, paths[index], paths[0])
		}
	}
	content, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read bridge: %v", err)
	}
	if string(content) != string(piBridgeSource) {
		t.Fatal("materialized bridge does not match embedded source")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(paths[0]), ".extension-*.tmp"))
	if err != nil {
		t.Fatalf("glob temp artifacts: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary bridge artifacts were not cleaned up: %v", matches)
	}
}

func TestPiBridgeMaterializerResolvesRelativeDataDir(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()

	workDir := t.TempDir()
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	}()

	manager, err := NewManager(Config{DataDir: "data"}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	path, err := manager.materializePiBridge()
	if err != nil {
		t.Fatalf("materializePiBridge: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("bridge path = %q, want absolute", path)
	}
	expectedRoot := filepath.Join(workDir, "data", "pi-bridge")
	if filepath.Dir(filepath.Dir(path)) != filepath.Clean(expectedRoot) {
		t.Fatalf("bridge path = %q, want path under %q", path, expectedRoot)
	}
}

func TestPiBridgeMaterializerAndProvenance(t *testing.T) {
	cleanup := initTestDB(t)
	defer cleanup()
	manager, err := NewManager(Config{DataDir: t.TempDir()}, zap.NewNop())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	path, err := manager.materializePiBridge()
	if err != nil {
		t.Fatalf("materializePiBridge: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Fatalf("bridge path = %q, want absolute", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bridge: %v", err)
	}
	if string(content) != string(piBridgeSource) {
		t.Fatal("materialized bridge does not match embedded source")
	}

	valid := piRPCSlashCommand{
		Name:   piBridgeCommandName,
		Source: "extension",
		SourceInfo: piRPCSourceInfo{
			Path: path, Source: "cli", Scope: "temporary", Origin: "top-level",
		},
	}
	if err := validatePiBridgeCommands([]piRPCSlashCommand{valid}, path); err != nil {
		t.Fatalf("valid provenance: %v", err)
	}
	for name, commands := range map[string][]piRPCSlashCommand{
		"missing":   nil,
		"duplicate": {valid, valid},
		"wrong command source": {{
			Name: piBridgeCommandName, Source: "prompt",
			SourceInfo: piRPCSourceInfo{Path: path, Source: "cli", Scope: "temporary", Origin: "top-level"},
		}},
		"wrong provenance source": {{
			Name: piBridgeCommandName, Source: "extension",
			SourceInfo: piRPCSourceInfo{Path: path, Source: "extension", Scope: "temporary", Origin: "top-level"},
		}},
		"wrong path": {{
			Name: piBridgeCommandName, Source: "extension",
			SourceInfo: piRPCSourceInfo{Path: filepath.Join(filepath.Dir(path), "other.ts"), Source: "cli", Scope: "temporary", Origin: "top-level"},
		}},
		"wrong scope": {{
			Name: piBridgeCommandName, Source: "extension",
			SourceInfo: piRPCSourceInfo{Path: path, Source: "cli", Scope: "project", Origin: "top-level"},
		}},
		"wrong origin": {{
			Name: piBridgeCommandName, Source: "extension",
			SourceInfo: piRPCSourceInfo{Path: path, Source: "cli", Scope: "temporary", Origin: "package"},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePiBridgeCommands(commands, path); err == nil {
				t.Fatal("expected provenance validation to fail")
			}
		})
	}

	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatalf("tamper bridge: %v", err)
	}
	if _, err := manager.materializePiBridge(); err == nil {
		t.Fatal("expected tampered bridge to fail closed")
	}
}
