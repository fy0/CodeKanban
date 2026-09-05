package process

import (
	"fmt"
	"strings"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sync/singleflight"
)

var (
	runtimeSnapshotCache = gocache.New(time.Second, 10*time.Second)
	// queryTimeout is the maximum time to wait for a process query
	queryTimeout      = 2 * time.Second
	runtimeQueryGroup singleflight.Group
)

// RuntimeSnapshot contains the process-tree state needed by terminal metadata.
type RuntimeSnapshot struct {
	PID               int32
	Status            string
	HasNonShellChild  bool
	ForegroundCommand string
	CapturedAt        time.Time
}

// GetRuntimeSnapshot returns one cached, singleflighted inspection of a process tree.
func GetRuntimeSnapshot(pid int32) RuntimeSnapshot {
	if pid <= 0 {
		return RuntimeSnapshot{PID: pid, Status: "unknown", CapturedAt: time.Now()}
	}

	cacheKey := fmt.Sprintf("runtime_%d", pid)
	if cached, found := runtimeSnapshotCache.Get(cacheKey); found {
		return cached.(RuntimeSnapshot)
	}

	value, _, _ := runtimeQueryGroup.Do(cacheKey, func() (any, error) {
		if cached, found := runtimeSnapshotCache.Get(cacheKey); found {
			return cached.(RuntimeSnapshot), nil
		}
		snapshot := inspectRuntimeSnapshot(pid)
		runtimeSnapshotCache.Set(cacheKey, snapshot, gocache.DefaultExpiration)
		return snapshot, nil
	})
	if snapshot, ok := value.(RuntimeSnapshot); ok {
		return snapshot
	}
	return RuntimeSnapshot{PID: pid, Status: "unknown", CapturedAt: time.Now()}
}

func inspectRuntimeSnapshot(pid int32) RuntimeSnapshot {
	result := make(chan RuntimeSnapshot, 1)
	go func() {
		snapshot := RuntimeSnapshot{PID: pid, Status: "unknown"}
		proc, err := process.NewProcess(pid)
		if err == nil {
			if _, statusErr := proc.Status(); statusErr == nil {
				snapshot.HasNonShellChild, snapshot.ForegroundCommand = findNonShellDescendant(pid, 0, 5)
				if snapshot.HasNonShellChild {
					snapshot.Status = "busy"
				} else {
					snapshot.Status = "idle"
				}
			}
		}
		if isShellCommand(snapshot.ForegroundCommand) {
			snapshot.ForegroundCommand = ""
		}
		snapshot.CapturedAt = time.Now()
		result <- snapshot
	}()

	select {
	case snapshot := <-result:
		return snapshot
	case <-time.After(queryTimeout):
		return RuntimeSnapshot{PID: pid, Status: "unknown", CapturedAt: time.Now()}
	}
}

// isShellCommand checks if a command line represents a shell command.
func isShellCommand(cmdline string) bool {
	cmdLower := strings.ToLower(cmdline)

	shellPatterns := []string{
		"bash.exe", "bash",
		"sh.exe", "/bin/sh",
		"zsh.exe", "zsh",
		"fish.exe", "fish",
		"cmd.exe",
		"powershell.exe",
		"pwsh.exe",
		"wsl.exe",
	}

	for _, pattern := range shellPatterns {
		if strings.Contains(cmdLower, pattern) {
			return true
		}
	}
	return false
}

// findNonShellDescendant returns whether a non-shell child exists and its command line.
func findNonShellDescendant(pid int32, depth, maxDepth int) (bool, string) {
	if pid <= 0 || depth >= maxDepth {
		return false, ""
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return false, ""
	}

	children, err := proc.Children()
	if err != nil || len(children) == 0 {
		return false, ""
	}

	foundNonShell := false
	for _, child := range children {
		if !isShellProcess(child) {
			foundNonShell = true
			if cmdline, cmdlineErr := child.Cmdline(); cmdlineErr == nil && cmdline != "" {
				return true, cmdline
			}
			continue
		}
		if found, cmdline := findNonShellDescendant(child.Pid, depth+1, maxDepth); found {
			foundNonShell = true
			if cmdline != "" {
				return true, cmdline
			}
		}
	}
	return foundNonShell, ""
}

// isShellProcess checks if a process is an intermediate shell that should be skipped.
func isShellProcess(proc *process.Process) bool {
	name, err := proc.Name()
	if err != nil {
		return false
	}

	name = strings.ToLower(name)

	// Common shell process names on Windows and Unix
	shellNames := []string{
		"bash", "bash.exe",
		"sh", "sh.exe",
		"zsh", "zsh.exe",
		"fish", "fish.exe",
		"cmd", "cmd.exe",
		"powershell", "powershell.exe",
		"pwsh", "pwsh.exe",
		"wsl", "wsl.exe",
		"conhost", "conhost.exe",
		"mintty", "mintty.exe", // Git Bash terminal
	}

	for _, shell := range shellNames {
		if name == shell {
			return true
		}
	}

	return false
}
