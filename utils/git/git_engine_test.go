package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutoRoutesAllOperationsToSystem(t *testing.T) {
	if !ProbeSystemGit(t.Context(), true).Available {
		t.Skip("system Git is unavailable")
	}
	previous := CurrentEngineSettings()
	ConfigureEngines(EngineSettings{Read: EnginePreferenceAuto, Write: EnginePreferenceAuto})
	t.Cleanup(func() { ConfigureEngines(previous) })

	repoPath := initTestRepo(t)
	repo, err := DetectRepository(repoPath)
	if err != nil {
		t.Fatalf("DetectRepository: %v", err)
	}
	defer repo.Close()

	for _, operation := range allGitOperations {
		if engine, err := repo.requireEngine(repoPath, operation); err != nil || engine != EngineSystem {
			t.Fatalf("%s engine = %q, err=%v, want system", operation, engine, err)
		}
	}
}

func TestExplicitEnginePreferencesDoNotFallback(t *testing.T) {
	previous := CurrentEngineSettings()
	t.Cleanup(func() { ConfigureEngines(previous) })
	repoPath := initTestRepo(t)

	t.Run("system read", func(t *testing.T) {
		ConfigureEngines(EngineSettings{
			Read:       EnginePreferenceSystem,
			Write:      EnginePreferenceSystem,
			Executable: filepath.Join(t.TempDir(), "missing-git"),
		})
		repo, err := DetectRepository(repoPath)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()
		if engine, err := repo.requireEngine(repoPath, OperationStatus); engine != EngineUnavailable || ErrorCode(err) != ErrorCodeSystemGitUnavailable {
			t.Fatalf("status engine = %q, err=%v, want unavailable system Git", engine, err)
		}
		if _, err := repo.GetCurrentBranch(); ErrorCode(err) != ErrorCodeSystemGitUnavailable {
			t.Fatalf("GetCurrentBranch error = %v, want unavailable system Git", err)
		}
		if _, err := repo.GetRemotes(); ErrorCode(err) != ErrorCodeSystemGitUnavailable {
			t.Fatalf("GetRemotes error = %v, want unavailable system Git", err)
		}
		if _, ok := repo.ConfigValue("core.autocrlf"); ok {
			t.Fatal("ConfigValue silently fell back from unavailable system Git")
		}
		if err := repo.ValidateBranchName("feature"); ErrorCode(err) != ErrorCodeSystemGitUnavailable {
			t.Fatalf("ValidateBranchName error = %v, want unavailable system Git", err)
		}
	})

	t.Run("builtin rebase", func(t *testing.T) {
		ConfigureEngines(EngineSettings{Read: EnginePreferenceAuto, Write: EnginePreferenceBuiltin})
		repo, err := DetectRepository(repoPath)
		if err != nil {
			t.Fatal(err)
		}
		defer repo.Close()
		if engine, err := repo.requireEngine(repoPath, OperationRebase); engine != EngineUnavailable || err == nil {
			t.Fatalf("rebase engine = %q, err=%v, want unavailable built-in operation", engine, err)
		}
	})
}

func TestAutoFallsBackToBuiltinWhenSystemGitIsUnavailable(t *testing.T) {
	previous := CurrentEngineSettings()
	ConfigureEngines(EngineSettings{
		Read:       EnginePreferenceAuto,
		Write:      EnginePreferenceAuto,
		Executable: filepath.Join(t.TempDir(), "missing-git"),
	})
	t.Cleanup(func() { ConfigureEngines(previous) })

	repoPath := initTestRepo(t)
	repo, err := DetectRepository(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if engine, err := repo.requireEngine(repoPath, OperationBranchesRead); err != nil || engine != EngineBuiltin {
		t.Fatalf("branches read engine = %q, err=%v, want builtin fallback", engine, err)
	}
	if engine, err := repo.requireEngine(repoPath, OperationStatus); err != nil || engine != EngineBuiltin {
		t.Fatalf("status engine = %q, err=%v, want builtin fallback", engine, err)
	}
	if engine, err := repo.requireEngine(repoPath, OperationDiff); err != nil || engine != EngineBuiltin {
		t.Fatalf("diff engine = %q, err=%v, want builtin fallback", engine, err)
	}
	if engine, err := repo.requireEngine(repoPath, OperationCommit); err != nil || engine != EngineBuiltin {
		t.Fatalf("commit engine = %q, err=%v, want builtin fallback", engine, err)
	}
}

func TestSystemGitStructuredOutputExcludesStderr(t *testing.T) {
	info := ProbeSystemGit(t.Context(), true)
	if !info.Available {
		t.Skip("system Git is unavailable")
	}
	previous := CurrentEngineSettings()
	ConfigureEngines(EngineSettings{Read: EnginePreferenceSystem, Write: EnginePreferenceSystem, Executable: info.Executable})
	t.Cleanup(func() { ConfigureEngines(previous) })

	output, err := runSystemGitOutput(
		context.Background(),
		"",
		OperationStatus,
		"-c",
		"alias.codex-output=!f() { printf expected; printf warning >&2; }; f",
		"codex-output",
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "expected" {
		t.Fatalf("structured output = %q, want stdout only", output)
	}
}

func TestSystemReadEngineProvidesStatusStatsAndDiff(t *testing.T) {
	info := ProbeSystemGit(t.Context(), true)
	if !info.Available {
		t.Skip("system Git is unavailable")
	}
	previous := CurrentEngineSettings()
	ConfigureEngines(EngineSettings{Read: EnginePreferenceSystem, Write: EnginePreferenceSystem, Executable: info.Executable})
	t.Cleanup(func() { ConfigureEngines(previous) })

	repoPath := initTestRepo(t)
	trackedPath := filepath.Join(repoPath, "README.md")
	if err := os.WriteFile(trackedPath, []byte("# Changed\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := ListFileStatusesFastContext(t.Context(), repoPath, true, 100)
	if err != nil {
		t.Fatalf("ListFileStatusesFastContext: %v", err)
	}
	if first.Statuses["README.md"].Kind != FileChangeKindModified {
		t.Fatalf("README status = %#v", first.Statuses["README.md"])
	}

	nextTime := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(trackedPath, []byte("# Changed\nthird!\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(trackedPath, nextTime, nextTime); err != nil {
		t.Fatal(err)
	}
	second, err := ListFileStatusesFastContext(t.Context(), repoPath, true, 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.ChangeToken == second.ChangeToken {
		t.Fatal("fast change token did not change after file content changed")
	}

	statuses := []FileStatus{second.Statuses["README.md"]}
	stats, err := GenerateDiffStatsAgainstHEADContext(t.Context(), repoPath, statuses)
	if err != nil {
		t.Fatalf("GenerateDiffStatsAgainstHEADContext: %v", err)
	}
	if stats["README.md"].Additions == 0 || stats["README.md"].Deletions == 0 {
		t.Fatalf("unexpected README stats: %#v", stats["README.md"])
	}
	diff, err := GenerateUnifiedDiffAgainstHEAD(repoPath, "README.md", "")
	if err != nil {
		t.Fatalf("GenerateUnifiedDiffAgainstHEAD: %v", err)
	}
	if diff == "" {
		t.Fatal("system Git returned an empty diff")
	}
}

func TestSystemCommitRunsHooks(t *testing.T) {
	info := ProbeSystemGit(t.Context(), true)
	if !info.Available {
		t.Skip("system Git is unavailable")
	}
	previous := CurrentEngineSettings()
	ConfigureEngines(EngineSettings{Read: EnginePreferenceBuiltin, Write: EnginePreferenceSystem, Executable: info.Executable})
	t.Cleanup(func() { ConfigureEngines(previous) })

	repoPath := initTestRepo(t)
	marker := filepath.Join(repoPath, "hook-ran.txt")
	hook := filepath.Join(repoPath, ".git", "hooks", "pre-commit")
	if err := writeTestHook(hook, marker); err != nil {
		t.Skipf("cannot install executable test hook: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "change.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	repo, err := DetectRepository(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.CommitAll(repoPath, "system commit"); err != nil {
		t.Fatalf("CommitAll: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("pre-commit hook did not run: %v", err)
	}
}

func writeTestHook(path, marker string) error {
	content := "#!/bin/sh\nprintf ran > " + shellQuote(marker) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	return os.Chmod(path, 0o755)
}

func shellQuote(value string) string {
	quoted := "'"
	for _, char := range value {
		if char == '\'' {
			quoted += "'\"'\"'"
		} else {
			quoted += string(char)
		}
	}
	return quoted + "'"
}
