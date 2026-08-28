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

// ProcessInfo contains basic information about a process.
type ProcessInfo struct {
	PID           int32   `json:"pid"`
	Name          string  `json:"name,omitempty"`
	Cmdline       string  `json:"cmdline,omitempty"`
	Status        string  `json:"status"`
	HasChildren   bool    `json:"hasChildren"`
	ChildrenCount int     `json:"childrenCount"`
	Children      []int32 `json:"children,omitempty"`
}

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

// GetProcessInfo retrieves information about a process by PID.
// Returns nil if the process doesn't exist or an error occurs.
func GetProcessInfo(pid int32) *ProcessInfo {
	if pid <= 0 {
		return nil
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}

	info := &ProcessInfo{
		PID:    pid,
		Status: "unknown",
	}

	// Get process name
	if name, err := proc.Name(); err == nil {
		info.Name = name
	}

	// Get command line
	if cmdline, err := proc.Cmdline(); err == nil {
		info.Cmdline = cmdline
	}

	// Get process status
	if statuses, err := proc.Status(); err == nil && len(statuses) > 0 {
		info.Status = statuses[0]
	}

	// Get children
	if children, err := proc.Children(); err == nil {
		info.ChildrenCount = len(children)
		info.HasChildren = len(children) > 0

		// Collect child PIDs
		info.Children = make([]int32, 0, len(children))
		for _, child := range children {
			info.Children = append(info.Children, child.Pid)
		}
	}

	return info
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

// GetDetailedProcessInfo returns comprehensive information about a process and its children.
func GetDetailedProcessInfo(pid int32) (*DetailedProcessInfo, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("invalid pid: %d", pid)
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("process not found: %w", err)
	}

	info := &DetailedProcessInfo{
		PID: pid,
	}

	// Get basic info
	if name, err := proc.Name(); err == nil {
		info.Name = name
	}

	if cmdline, err := proc.Cmdline(); err == nil {
		info.Cmdline = cmdline
	}

	if statuses, err := proc.Status(); err == nil && len(statuses) > 0 {
		info.Status = statuses[0]
	}

	// Get children details
	children, err := proc.Children()
	if err == nil && len(children) > 0 {
		info.HasChildren = true
		info.ChildrenCount = len(children)
		info.Children = make([]ChildProcessInfo, 0, len(children))

		for _, child := range children {
			childInfo := ChildProcessInfo{
				PID: child.Pid,
			}

			if name, err := child.Name(); err == nil {
				childInfo.Name = name
			}

			if cmdline, err := child.Cmdline(); err == nil {
				childInfo.Cmdline = cmdline
			}

			info.Children = append(info.Children, childInfo)
		}
	}

	return info, nil
}

// DetailedProcessInfo contains comprehensive information about a process.
type DetailedProcessInfo struct {
	PID           int32              `json:"pid"`
	Name          string             `json:"name,omitempty"`
	Cmdline       string             `json:"cmdline,omitempty"`
	Status        string             `json:"status,omitempty"`
	HasChildren   bool               `json:"hasChildren"`
	ChildrenCount int                `json:"childrenCount"`
	Children      []ChildProcessInfo `json:"children,omitempty"`
}

// ChildProcessInfo contains basic information about a child process.
type ChildProcessInfo struct {
	PID     int32  `json:"pid"`
	Name    string `json:"name,omitempty"`
	Cmdline string `json:"cmdline,omitempty"`
}

// AIProcessInfo contains information about a detected AI assistant process.
type AIProcessInfo struct {
	PID        int32     // Process ID of the AI assistant
	Cmdline    string    // Command line of the AI assistant
	Cwd        string    // Current working directory
	CreateTime time.Time // Process creation time
}

// FindAIAssistantProcess searches for an AI assistant process in the process tree
// and returns its detailed information including Cwd and CreateTime.
// The cmdlineChecker function should return true if the cmdline matches an AI assistant.
func FindAIAssistantProcess(rootPID int32, cmdlineChecker func(cmdline string) bool) *AIProcessInfo {
	if rootPID <= 0 || cmdlineChecker == nil {
		return nil
	}

	// Query with timeout
	result := make(chan *AIProcessInfo, 1)
	go func() {
		info := findAIAssistantRecursive(rootPID, cmdlineChecker, 0, 5)
		result <- info
	}()

	select {
	case info := <-result:
		return info
	case <-time.After(queryTimeout):
		return nil
	}
}

// findAIAssistantRecursive recursively searches for an AI assistant in the process tree.
func findAIAssistantRecursive(pid int32, cmdlineChecker func(cmdline string) bool, depth, maxDepth int) *AIProcessInfo {
	if pid <= 0 || depth >= maxDepth {
		return nil
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}

	// Get children first
	children, err := proc.Children()
	if err != nil || len(children) == 0 {
		return nil
	}

	// Check each child process
	for _, child := range children {
		cmdline, err := child.Cmdline()
		if err != nil || cmdline == "" {
			continue
		}

		// Check if this is an AI assistant
		if cmdlineChecker(cmdline) {
			info := &AIProcessInfo{
				PID:     child.Pid,
				Cmdline: cmdline,
			}

			// Get current working directory
			if cwd, err := child.Cwd(); err == nil {
				info.Cwd = cwd
			}

			// Get process creation time
			if createTime, err := child.CreateTime(); err == nil {
				info.CreateTime = time.UnixMilli(createTime)
			} else {
				// Fallback to current time if we can't get creation time
				info.CreateTime = time.Now()
			}

			return info
		}

		// If this is a shell process, recurse into its children
		if isShellProcess(child) {
			if result := findAIAssistantRecursive(child.Pid, cmdlineChecker, depth+1, maxDepth); result != nil {
				return result
			}
		}
	}

	return nil
}
