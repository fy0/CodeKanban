package process

import (
	"fmt"
	"strings"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/shirou/gopsutil/v4/process"
)

var (
	// processCache caches process query results to avoid repeated expensive system calls
	processCache = gocache.New(3*time.Second, 10*time.Second)
	// queryTimeout is the maximum time to wait for a process query
	queryTimeout = 2 * time.Second
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

// GetForegroundCommand attempts to get the foreground process command.
// For a shell, this recursively searches the process tree to find the actual
// running command (handles multi-layer shell structure like PowerShell).
// Returns the command line of the running command, or empty string if not found.
// Note: This may not work reliably for Git Bash where child processes detach.
func GetForegroundCommand(pid int32) string {
	if pid <= 0 {
		return ""
	}

	// Check cache first
	cacheKey := fmt.Sprintf("fg_cmd_%d", pid)
	if cached, found := processCache.Get(cacheKey); found {
		return cached.(string)
	}

	// Query with timeout
	result := make(chan string, 1)
	go func() {
		cmd := findForegroundCommandRecursive(pid, 0, 5)
		if cmd != "" && !isShellCommand(cmd) {
			result <- cmd
			return
		}
		result <- ""
	}()

	select {
	case cmd := <-result:
		processCache.Set(cacheKey, cmd, gocache.DefaultExpiration)
		return cmd
	case <-time.After(queryTimeout):
		// Timeout - cache empty result to avoid repeated slow queries
		processCache.Set(cacheKey, "", gocache.DefaultExpiration)
		return ""
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

// findForegroundCommandRecursive recursively searches for the actual running command.
// It skips intermediate shell processes (bash, sh, cmd, powershell) and returns
// the first non-shell command found in the process tree.
// Uses gopsutil's Children() which is faster than manual Ppid traversal.
func findForegroundCommandRecursive(pid int32, depth, maxDepth int) string {
	if pid <= 0 || depth >= maxDepth {
		return ""
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return ""
	}

	// Use gopsutil's Children() - faster than manual Ppid traversal
	children, err := proc.Children()
	if err != nil || len(children) == 0 {
		return ""
	}

	// Check each child process
	for _, child := range children {
		cmdline, err := child.Cmdline()
		if err != nil || cmdline == "" {
			continue
		}

		// If this is not a shell process, return its command line
		if !isShellProcess(child) {
			return cmdline
		}

		// This is a shell process, recurse into its children
		if result := findForegroundCommandRecursive(child.Pid, depth+1, maxDepth); result != "" {
			return result
		}
	}

	// Fallback: return the first child's command line if no non-shell found
	if len(children) > 0 {
		if cmdline, err := children[0].Cmdline(); err == nil {
			return cmdline
		}
	}

	return ""
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

// IsProcessBusy checks if a process is running a non-shell command.
// This recursively checks the process tree to handle Git Bash multi-layer structure.
// Returns true only if there's a non-shell child process running.
func IsProcessBusy(pid int32) bool {
	if pid <= 0 {
		return false
	}

	// Check cache first
	cacheKey := fmt.Sprintf("busy_%d", pid)
	if cached, found := processCache.Get(cacheKey); found {
		return cached.(bool)
	}

	// Query with timeout
	result := make(chan bool, 1)
	go func() {
		busy := hasNonShellChild(pid, 0, 5)
		result <- busy
	}()

	select {
	case busy := <-result:
		processCache.Set(cacheKey, busy, gocache.DefaultExpiration)
		return busy
	case <-time.After(queryTimeout):
		// Timeout - assume not busy
		processCache.Set(cacheKey, false, gocache.DefaultExpiration)
		return false
	}
}

// hasNonShellChild recursively checks if there's any non-shell child process.
// Uses gopsutil's Children() which is faster than manual Ppid traversal.
func hasNonShellChild(pid int32, depth, maxDepth int) bool {
	if pid <= 0 || depth >= maxDepth {
		return false
	}

	proc, err := process.NewProcess(pid)
	if err != nil {
		return false
	}

	children, err := proc.Children()
	if err != nil || len(children) == 0 {
		return false
	}

	for _, child := range children {
		if !isShellProcess(child) {
			return true
		}
		// Recurse into shell children
		if hasNonShellChild(child.Pid, depth+1, maxDepth) {
			return true
		}
	}

	return false
}

// GetProcessStatus returns a simple status string: "idle", "busy", or "unknown".
// Uses recursive check to handle Git Bash multi-layer shell structure.
func GetProcessStatus(pid int32) string {
	if pid <= 0 {
		return "unknown"
	}

	// Check cache first
	cacheKey := fmt.Sprintf("status_%d", pid)
	if cached, found := processCache.Get(cacheKey); found {
		return cached.(string)
	}

	// Query with timeout
	result := make(chan string, 1)
	go func() {
		proc, err := process.NewProcess(pid)
		if err != nil {
			result <- "unknown"
			return
		}

		// Check if process exists
		_, err = proc.Status()
		if err != nil {
			result <- "unknown"
			return
		}

		// Use recursive check for non-shell children
		if hasNonShellChild(pid, 0, 5) {
			result <- "busy"
		} else {
			result <- "idle"
		}
	}()

	select {
	case status := <-result:
		processCache.Set(cacheKey, status, gocache.DefaultExpiration)
		return status
	case <-time.After(queryTimeout):
		// Timeout - return unknown
		processCache.Set(cacheKey, "unknown", gocache.DefaultExpiration)
		return "unknown"
	}
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
