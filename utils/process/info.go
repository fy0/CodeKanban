package process

import (
	"fmt"
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
	PID           int32    `json:"pid"`
	Name          string   `json:"name,omitempty"`
	Cmdline       string   `json:"cmdline,omitempty"`
	Status        string   `json:"status"`
	HasChildren   bool     `json:"hasChildren"`
	ChildrenCount int      `json:"childrenCount"`
	Children      []int32  `json:"children,omitempty"`
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
// For a shell, this tries to find the most recently created child process.
// Returns the command line of the child, or empty string if no child is found.
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
		proc, err := process.NewProcess(pid)
		if err != nil {
			result <- ""
			return
		}

		children, err := proc.Children()
		if err != nil || len(children) == 0 {
			result <- ""
			return
		}

		// Get the first child's command (simple heuristic)
		// In a real scenario, you might want to find the foreground process group
		if len(children) > 0 {
			if cmdline, err := children[0].Cmdline(); err == nil {
				result <- cmdline
				return
			}
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

// IsProcessBusy checks if a process has any child processes.
// This is useful for determining if a shell is running a command.
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
		proc, err := process.NewProcess(pid)
		if err != nil {
			result <- false
			return
		}

		children, err := proc.Children()
		if err != nil {
			result <- false
			return
		}

		result <- len(children) > 0
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

// GetProcessStatus returns a simple status string: "idle", "busy", or "unknown".
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

		// Check if process has children
		children, err := proc.Children()
		if err != nil {
			result <- "unknown"
			return
		}

		if len(children) > 0 {
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
