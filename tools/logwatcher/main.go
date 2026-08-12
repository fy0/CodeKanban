package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"go.uber.org/zap"

	ai_assistant2 "code-kanban/utils/ai_assistant2"
	"code-kanban/utils/ai_assistant2/log_watcher"
	"code-kanban/utils/ai_assistant2/types"
)

type WatcherSession struct {
	watcher       *log_watcher.LogWatcher
	assistantType types.AssistantType
	workingDir    string
	startTime     time.Time
	searchMode    log_watcher.SearchMode
	foundBy       string // "new_session", "resumed_session", or "session_id"
	messages      []*log_watcher.UserMessage
	messagesMu    sync.RWMutex
	lastQueryIdx  int
}

// parseSearchMode parses a search mode string
func parseSearchMode(s string) (log_watcher.SearchMode, bool) {
	switch strings.ToLower(s) {
	case "both", "":
		return log_watcher.SearchModeBoth, true
	case "ctime", "create", "creation":
		return log_watcher.SearchModeCreationOnly, true
	case "mtime", "mod", "modify", "modification":
		return log_watcher.SearchModeModificationOnly, true
	default:
		return log_watcher.SearchModeBoth, false
	}
}

func main() {
	fmt.Println("=== LogWatcher Test Tool ===")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	var session *WatcherSession
	var ctx context.Context
	var cancel context.CancelFunc

	for {
		fmt.Print("\n> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Printf("Error reading input: %v\n", err)
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		parts := strings.Fields(input)
		cmd := strings.ToLower(parts[0])

		switch cmd {
		case "help", "h", "?":
			printHelp()

		case "watch":
			// watch <type> <workingDir> [mode] - directly start watching with specified type and path
			if len(parts) < 3 {
				fmt.Println("Usage: watch <codex|claude|pi> <workingDir> [mode]")
				fmt.Println("       mode: both (default), ctime, mtime")
				fmt.Println("Example: watch claude D:\\codes\\2025\\aicode-kanban")
				fmt.Println("Example: watch claude D:\\codes\\2025\\aicode-kanban mtime")
				continue
			}

			var aType types.AssistantType
			switch strings.ToLower(parts[1]) {
			case "codex":
				aType = types.AssistantTypeCodex
			case "claude", "claudecode":
				aType = types.AssistantTypeClaudeCode
			case "pi":
				aType = types.AssistantTypePi
			default:
				fmt.Printf("Unknown type: %s (use 'codex', 'claude', or 'pi')\n", parts[1])
				continue
			}

			// Check if last part is a mode
			searchMode := log_watcher.SearchModeBoth
			workingDirParts := parts[2:]
			if len(parts) >= 4 {
				lastPart := parts[len(parts)-1]
				if mode, ok := parseSearchMode(lastPart); ok && (lastPart == "both" || lastPart == "ctime" || lastPart == "mtime") {
					searchMode = mode
					workingDirParts = parts[2 : len(parts)-1]
				}
			}
			workingDir := strings.Join(workingDirParts, " ")

			// Stop existing watcher if any
			if cancel != nil {
				cancel()
			}

			session, ctx, cancel = startWatcherWithMode(aType, workingDir, time.Now().Add(-time.Hour), searchMode)
			if session != nil {
				fmt.Printf("Watcher started for %s\n", session.assistantType.DisplayName())
				fmt.Printf("Working directory: %s\n", session.workingDir)
				fmt.Printf("Search mode: %s\n", session.searchMode.String())
				if session.foundBy != "" {
					fmt.Printf("Found by: %s\n", session.foundBy)
				}
			}

		case "session":
			// session <type> <path> <id> - Watch a specific session by ID
			if len(parts) < 4 {
				fmt.Println("Usage: session <codex|claude|pi> <workingDir> <sessionID>")
				fmt.Println("Example: session claude D:\\codes\\2025\\aicode-kanban 8a874861-cbd9-4c66-964d-0b9311c68598")
				continue
			}

			var aType types.AssistantType
			switch strings.ToLower(parts[1]) {
			case "codex":
				aType = types.AssistantTypeCodex
			case "claude", "claudecode":
				aType = types.AssistantTypeClaudeCode
			case "pi":
				aType = types.AssistantTypePi
			default:
				fmt.Printf("Unknown type: %s (use 'codex', 'claude', or 'pi')\n", parts[1])
				continue
			}

			sessionID := parts[len(parts)-1]
			workingDir := strings.Join(parts[2:len(parts)-1], " ")

			// Stop existing watcher if any
			if cancel != nil {
				cancel()
			}

			session, ctx, cancel = startWatcherBySessionID(aType, workingDir, sessionID)
			if session != nil {
				fmt.Printf("Watcher started for %s\n", session.assistantType.DisplayName())
				fmt.Printf("Working directory: %s\n", session.workingDir)
				fmt.Printf("Session ID: %s\n", sessionID)
				fmt.Printf("Found by: %s\n", session.foundBy)
			}

		case "pid":
			if len(parts) < 2 {
				fmt.Println("Usage: pid <PID> [mode]")
				fmt.Println("       mode: both (default), ctime, mtime")
				continue
			}
			pid, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil {
				fmt.Printf("Invalid PID: %v\n", err)
				continue
			}

			// Check for mode parameter
			searchMode := log_watcher.SearchModeBoth
			if len(parts) >= 3 {
				if mode, ok := parseSearchMode(parts[2]); ok {
					searchMode = mode
				} else {
					fmt.Printf("Unknown mode: %s (use 'both', 'ctime', or 'mtime')\n", parts[2])
					continue
				}
			}

			// Stop existing watcher if any
			if cancel != nil {
				cancel()
			}

			session, ctx, cancel = startWatcherWithPID(int32(pid), searchMode)
			if session != nil {
				fmt.Printf("Watcher started for %s\n", session.assistantType.DisplayName())
				fmt.Printf("Working directory: %s\n", session.workingDir)
				fmt.Printf("Search mode: %s\n", session.searchMode.String())
				if session.foundBy != "" {
					fmt.Printf("Found by: %s\n", session.foundBy)
				}
			}

		case "scan":
			if len(parts) < 2 {
				fmt.Println("Usage: scan <PID>")
				fmt.Println("Scans the PID and its children for AI assistants")
				continue
			}
			pid, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil {
				fmt.Printf("Invalid PID: %v\n", err)
				continue
			}
			scanProcessTree(int32(pid))

		case "latest", "l":
			if session == nil {
				fmt.Println("No active session. Use 'pid <PID>' first.")
				continue
			}
			showLatestMessage(session)

		case "new", "n":
			if session == nil {
				fmt.Println("No active session. Use 'pid <PID>' first.")
				continue
			}
			showNewMessages(session)

		case "all", "a":
			if session == nil {
				fmt.Println("No active session. Use 'pid <PID>' first.")
				continue
			}
			showAllMessages(session)

		case "info", "i":
			if session == nil {
				fmt.Println("No active session. Use 'pid <PID>' first.")
				continue
			}
			showInfo(session)

		case "reload", "r":
			if session == nil {
				fmt.Println("No active session. Use 'pid <PID>' first.")
				continue
			}
			// Stop existing watcher
			if cancel != nil {
				cancel()
			}
			// Restart with same parameters
			session, ctx, cancel = startWatcherWithMode(session.assistantType, session.workingDir, session.startTime, session.searchMode)
			if session != nil {
				fmt.Println("Watcher reloaded.")
			}

		case "encode":
			// Test path encoding
			if len(parts) < 2 {
				fmt.Println("Usage: encode <path>")
				continue
			}
			path := strings.Join(parts[1:], " ")
			encoded := log_watcher.EncodePathForClaude(path)
			fmt.Printf("Original: %s\n", path)
			fmt.Printf("Encoded:  %s\n", encoded)
			fmt.Printf("Full path: C:\\Users\\test\\.claude\\projects\\%s\n", encoded)

		case "chat", "c":
			// Show chat history from session file
			if session == nil {
				fmt.Println("No active session. Use 'pid <PID>' or 'session' first.")
				continue
			}
			limit := 10 // default
			if len(parts) >= 2 {
				if n, err := strconv.Atoi(parts[1]); err == nil && n > 0 {
					limit = n
				}
			}
			showChatHistory(session, limit)

		case "chatfile":
			// Directly read a session file and show chat
			if len(parts) < 2 {
				fmt.Println("Usage: chatfile <path> [limit]")
				fmt.Println("Example: chatfile C:\\Users\\test\\.claude\\projects\\D--codes-2025-aicode-kanban\\xxx.jsonl 20")
				continue
			}
			filePath := parts[1]
			limit := 10
			if len(parts) >= 3 {
				if n, err := strconv.Atoi(parts[2]); err == nil && n > 0 {
					limit = n
				}
			}
			showChatFromFile(filePath, limit)

		case "quit", "exit", "q":
			if cancel != nil {
				cancel()
			}
			fmt.Println("Goodbye!")
			return

		default:
			fmt.Printf("Unknown command: %s\n", cmd)
			fmt.Println("Type 'help' for available commands.")
		}

		// Keep context reference for later cleanup
		_ = ctx
	}
}

func printHelp() {
	fmt.Println(`Available commands:
  watch <type> <path> [mode] - Start watching with specified type and working directory
                               type: codex, claude, pi
                               mode: both (default), ctime, mtime
                               Example: watch claude D:\codes\2025\aicode-kanban
                               Example: watch claude D:\codes\2025\aicode-kanban mtime
  session <type> <path> <id> - Watch a specific session by ID (skips file search)
                               Example: session claude D:\codes\2025\aicode-kanban 8a874861-cbd9-4c66-964d-0b9311c68598
  pid <PID> [mode]           - Find AI assistant in process tree and start watching
                               mode: both (default), ctime, mtime
  scan <PID>                 - Scan process tree for AI assistants (without starting watcher)
  latest (l)                 - Show the latest user message
  new (n)                    - Show new messages since last query
  all (a)                    - Show all captured messages
  chat [n] (c)               - Show last n chat messages (default 10), user & assistant
  chatfile <path> [n]        - Read session file and show chat (without watching)
  info (i)                   - Show watcher info (lines read, offset, etc.)
  reload (r)                 - Reload the watcher (reset offset and re-read from start)
  encode <path>              - Test path encoding for Claude Code
  quit (q)                   - Exit the tool
  help (h)                   - Show this help

Search modes:
  both  - Try creation time first, then modification time (default)
  ctime - Only search by creation time (new sessions)
  mtime - Only search by modification time (resumed sessions)`)
}

func scanProcessTree(rootPID int32) {
	fmt.Printf("Scanning process tree from PID %d...\n\n", rootPID)

	detector := ai_assistant2.NewAssistantDetector()

	var scan func(pid int32, depth int)
	scan = func(pid int32, depth int) {
		proc, err := process.NewProcess(pid)
		if err != nil {
			return
		}

		cmdline, err := proc.Cmdline()
		if err != nil {
			cmdline = "(unable to get cmdline)"
		}

		name, _ := proc.Name()
		indent := strings.Repeat("  ", depth)

		// Check if this is an AI assistant
		info := detector.DetectFromCommand(cmdline)
		if info != nil {
			fmt.Printf("%s[%d] %s - %s ✓ %s\n", indent, pid, name, truncate(cmdline, 60), info.Type.DisplayName())
		} else {
			fmt.Printf("%s[%d] %s - %s\n", indent, pid, name, truncate(cmdline, 60))
		}

		// Scan children
		children, err := proc.Children()
		if err == nil {
			for _, child := range children {
				scan(child.Pid, depth+1)
			}
		}
	}

	scan(rootPID, 0)
}

func findAIAssistantInTree(rootPID int32) (types.AssistantType, string, int32, time.Time) {
	detector := ai_assistant2.NewAssistantDetector()

	var find func(pid int32) (types.AssistantType, string, int32, time.Time)
	find = func(pid int32) (types.AssistantType, string, int32, time.Time) {
		proc, err := process.NewProcess(pid)
		if err != nil {
			return types.AssistantTypeUnknown, "", 0, time.Time{}
		}

		cmdline, err := proc.Cmdline()
		if err != nil {
			cmdline = ""
		}

		// Check if this is an AI assistant
		info := detector.DetectFromCommand(cmdline)
		if info != nil {
			// Get working directory
			cwd, err := proc.Cwd()
			if err != nil {
				cwd = ""
			}

			// Get process start time
			createTime, err := proc.CreateTime()
			startTime := time.Now()
			if err == nil {
				startTime = time.UnixMilli(createTime)
			}

			return info.Type, cwd, pid, startTime
		}

		// Check children
		children, err := proc.Children()
		if err == nil {
			for _, child := range children {
				aType, cwd, foundPid, startTime := find(child.Pid)
				if aType != types.AssistantTypeUnknown {
					return aType, cwd, foundPid, startTime
				}
			}
		}

		return types.AssistantTypeUnknown, "", 0, time.Time{}
	}

	return find(rootPID)
}

func startWatcherWithPID(pid int32, searchMode log_watcher.SearchMode) (*WatcherSession, context.Context, context.CancelFunc) {
	fmt.Printf("Searching for AI assistant in process tree of PID %d...\n", pid)

	aType, workingDir, foundPID, startTime := findAIAssistantInTree(pid)
	if aType == types.AssistantTypeUnknown {
		fmt.Println("No AI assistant found in process tree.")
		return nil, nil, nil
	}

	fmt.Printf("Found: %s (PID: %d)\n", aType.DisplayName(), foundPID)
	fmt.Printf("Working directory: %s\n", workingDir)
	fmt.Printf("Process start time: %s\n", startTime.Format(time.RFC3339))

	return startWatcherWithMode(aType, workingDir, startTime, searchMode)
}

func startWatcherWithMode(aType types.AssistantType, workingDir string, startTime time.Time, searchMode log_watcher.SearchMode) (*WatcherSession, context.Context, context.CancelFunc) {
	session := &WatcherSession{
		assistantType: aType,
		workingDir:    workingDir,
		startTime:     startTime,
		searchMode:    searchMode,
		messages:      make([]*log_watcher.UserMessage, 0),
	}

	logger, _ := zap.NewDevelopment()

	callback := func(event log_watcher.WatcherEvent) {
		switch event.Type {
		case log_watcher.EventTypeSessionFound:
			fmt.Println("\n[Event] Session file found!")
		case log_watcher.EventTypeNewMessage:
			if event.Message != nil {
				session.messagesMu.Lock()
				session.messages = append(session.messages, event.Message)
				session.messagesMu.Unlock()
				fmt.Printf("\n[Event] New message: %s\n> ", truncate(event.Message.Message, 50))
			}
		case log_watcher.EventTypeError:
			fmt.Printf("\n[Event] Error: %v\n> ", event.Error)
		case log_watcher.EventTypeStopped:
			fmt.Println("\n[Event] Watcher stopped")
		}
	}

	// For Claude Code, we can get search result details
	if aType == types.AssistantTypeClaudeCode {
		searcher, err := log_watcher.NewClaudeCodeFileSearcher(workingDir)
		if err != nil {
			fmt.Printf("Failed to create searcher: %v\n", err)
			return nil, nil, nil
		}
		searcher.SetSearchMode(searchMode)

		// Try to find the file first to get the foundBy info
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := searcher.FindSessionFileWithResult(ctx, startTime)
		cancel()
		if err != nil {
			fmt.Printf("Error searching for session file: %v\n", err)
		} else if result.FilePath != "" {
			session.foundBy = result.FoundBy
			fmt.Printf("Session file: %s\n", result.FilePath)
			fmt.Printf("Created: %s\n", result.CreateTime.Format(time.RFC3339))
			fmt.Printf("Modified: %s\n", result.ModTime.Format(time.RFC3339))
		}
	}

	watcher, err := log_watcher.CreateWatcherForAssistantWithWorkingDirAndMode(
		aType,
		startTime,
		workingDir,
		searchMode,
		logger,
		callback,
	)

	if err != nil {
		fmt.Printf("Failed to create watcher: %v\n", err)
		return nil, nil, nil
	}

	if watcher == nil {
		fmt.Println("Watcher not supported for this assistant type or missing working directory.")
		return nil, nil, nil
	}

	session.watcher = watcher

	ctx, cancel := context.WithCancel(context.Background())
	if err := watcher.Start(ctx); err != nil {
		fmt.Printf("Failed to start watcher: %v\n", err)
		cancel()
		return nil, nil, nil
	}

	// Wait a bit for initial file discovery
	time.Sleep(2 * time.Second)

	// Get initial messages from watcher
	info := watcher.Info()
	if info.UserMessages != nil {
		session.messagesMu.Lock()
		session.messages = info.UserMessages
		session.messagesMu.Unlock()
	}

	return session, ctx, cancel
}

func startWatcherBySessionID(aType types.AssistantType, workingDir string, sessionID string) (*WatcherSession, context.Context, context.CancelFunc) {
	session := &WatcherSession{
		assistantType: aType,
		workingDir:    workingDir,
		startTime:     time.Now(),
		foundBy:       "session_id",
		messages:      make([]*log_watcher.UserMessage, 0),
	}

	// Find the session file by ID
	var filePath string
	switch aType {
	case types.AssistantTypeClaudeCode:
		searcher, err := log_watcher.NewClaudeCodeFileSearcher(workingDir)
		if err != nil {
			fmt.Printf("Failed to create searcher: %v\n", err)
			return nil, nil, nil
		}
		filePath, err = searcher.FindBySessionID(sessionID)
		if err != nil {
			fmt.Printf("Error finding session file: %v\n", err)
			return nil, nil, nil
		}
	case types.AssistantTypePi:
		searcher, err := log_watcher.NewPiFileSearcherWithWorkingDir(workingDir)
		if err != nil {
			fmt.Printf("Failed to create searcher: %v\n", err)
			return nil, nil, nil
		}
		filePath, err = searcher.FindBySessionID(context.Background(), sessionID)
		if err != nil {
			fmt.Printf("Error finding session file: %v\n", err)
			return nil, nil, nil
		}
	default:
		fmt.Println("Session ID lookup is supported for Claude Code and Pi.")
		return nil, nil, nil
	}

	if filePath == "" {
		fmt.Printf("Session file not found for ID: %s\n", sessionID)
		return nil, nil, nil
	}

	fmt.Printf("Found session file: %s\n", filePath)

	logger, _ := zap.NewDevelopment()

	callback := func(event log_watcher.WatcherEvent) {
		switch event.Type {
		case log_watcher.EventTypeSessionFound:
			fmt.Println("\n[Event] Session file found!")
		case log_watcher.EventTypeNewMessage:
			if event.Message != nil {
				session.messagesMu.Lock()
				session.messages = append(session.messages, event.Message)
				session.messagesMu.Unlock()
				fmt.Printf("\n[Event] New message: %s\n> ", truncate(event.Message.Message, 50))
			}
		case log_watcher.EventTypeError:
			fmt.Printf("\n[Event] Error: %v\n> ", event.Error)
		case log_watcher.EventTypeStopped:
			fmt.Println("\n[Event] Watcher stopped")
		}
	}

	watcher, err := log_watcher.CreateWatcherWithFile(
		aType,
		filePath,
		logger,
		callback,
	)

	if err != nil {
		fmt.Printf("Failed to create watcher: %v\n", err)
		return nil, nil, nil
	}

	if watcher == nil {
		fmt.Println("Watcher not supported for this assistant type.")
		return nil, nil, nil
	}

	session.watcher = watcher

	ctx, cancel := context.WithCancel(context.Background())
	if err := watcher.Start(ctx); err != nil {
		fmt.Printf("Failed to start watcher: %v\n", err)
		cancel()
		return nil, nil, nil
	}

	// Wait a bit for initial file reading
	time.Sleep(1 * time.Second)

	// Get initial messages from watcher
	info := watcher.Info()
	if info.UserMessages != nil {
		session.messagesMu.Lock()
		session.messages = info.UserMessages
		session.messagesMu.Unlock()
	}

	return session, ctx, cancel
}

func showLatestMessage(session *WatcherSession) {
	info := session.watcher.Info()

	if info.LastMessage == nil {
		fmt.Println("No messages captured yet.")
		return
	}

	fmt.Printf("\n--- Latest Message ---\n")
	fmt.Printf("Time: %s\n", info.LastMessage.Timestamp.Format(time.RFC3339))
	fmt.Printf("Content:\n%s\n", info.LastMessage.Message)
	fmt.Printf("----------------------\n")
}

func showNewMessages(session *WatcherSession) {
	session.messagesMu.RLock()
	totalMessages := len(session.messages)
	lastIdx := session.lastQueryIdx
	session.messagesMu.RUnlock()

	if totalMessages == 0 {
		fmt.Println("No messages captured yet.")
		return
	}

	if lastIdx >= totalMessages {
		fmt.Println("No new messages since last query.")
		return
	}

	session.messagesMu.RLock()
	newMessages := session.messages[lastIdx:]
	session.messagesMu.RUnlock()

	fmt.Printf("\n--- New Messages (%d) ---\n", len(newMessages))
	for i, msg := range newMessages {
		fmt.Printf("[%d] %s\n", lastIdx+i+1, msg.Timestamp.Format(time.RFC3339))
		fmt.Printf("    %s\n", truncate(msg.Message, 100))
	}
	fmt.Printf("-------------------------\n")

	session.messagesMu.Lock()
	session.lastQueryIdx = totalMessages
	session.messagesMu.Unlock()
}

func showAllMessages(session *WatcherSession) {
	session.messagesMu.RLock()
	messages := session.messages
	session.messagesMu.RUnlock()

	if len(messages) == 0 {
		fmt.Println("No messages captured yet.")
		return
	}

	fmt.Printf("\n--- All Messages (%d) ---\n", len(messages))
	for i, msg := range messages {
		fmt.Printf("[%d] %s\n", i+1, msg.Timestamp.Format(time.RFC3339))
		fmt.Printf("    %s\n", truncate(msg.Message, 100))
	}
	fmt.Printf("-------------------------\n")
}

func showInfo(session *WatcherSession) {
	info := session.watcher.Info()

	fmt.Printf("\n--- Watcher Info ---\n")
	fmt.Printf("State:        %s\n", info.State)
	fmt.Printf("Session ID:   %s\n", info.SessionID)
	fmt.Printf("File Path:    %s\n", info.FilePath)
	fmt.Printf("Lines Read:   %d\n", info.LinesRead)
	fmt.Printf("File Offset:  %d bytes\n", info.FileOffset)
	fmt.Printf("Last Check:   %s\n", info.LastCheckTime.Format(time.RFC3339))
	fmt.Printf("Messages:     %d\n", info.MessageCount)

	if info.SessionMeta != nil {
		fmt.Printf("\n--- Session Meta ---\n")
		fmt.Printf("ID:           %s\n", info.SessionMeta.ID)
		fmt.Printf("Cwd:          %s\n", info.SessionMeta.Cwd)
		fmt.Printf("Originator:   %s\n", info.SessionMeta.Originator)
		fmt.Printf("CLI Version:  %s\n", info.SessionMeta.CliVersion)
		fmt.Printf("Model:        %s\n", info.SessionMeta.Model)
	}

	if info.Error != "" {
		fmt.Printf("\nError: %s\n", info.Error)
	}
	fmt.Printf("--------------------\n")
}

func truncate(s string, maxLen int) string {
	// Remove newlines for display
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ChatMessage represents a message in the chat history
type ChatMessage struct {
	Role      string    // "user" or "assistant"
	Content   string    // The message content
	Timestamp time.Time // When the message was sent
	Model     string    // Model used (for assistant messages)
}

// showChatHistory shows the chat history from the current session
func showChatHistory(session *WatcherSession, limit int) {
	info := session.watcher.Info()
	if info.FilePath == "" {
		fmt.Println("No session file found.")
		return
	}
	showChatFromFile(info.FilePath, limit)
}

// showChatFromFile reads a session file and displays the chat history
func showChatFromFile(filePath string, limit int) {
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()

	var messages []ChatMessage
	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		msg := parseChatLine(line)
		if msg != nil {
			messages = append(messages, *msg)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	if len(messages) == 0 {
		fmt.Println("No chat messages found.")
		return
	}

	// Show last N messages
	start := 0
	if len(messages) > limit {
		start = len(messages) - limit
	}

	fmt.Printf("\n=== Chat History (showing %d of %d messages) ===\n\n", len(messages)-start, len(messages))
	for i := start; i < len(messages); i++ {
		msg := messages[i]
		timeStr := msg.Timestamp.Format("15:04:05")

		if msg.Role == "user" {
			fmt.Printf("┌─ 👤 User [%s]\n", timeStr)
			printWrapped(msg.Content, "│ ", 100)
			fmt.Println("└────────────────────")
		} else {
			modelInfo := ""
			if msg.Model != "" {
				modelInfo = fmt.Sprintf(" (%s)", msg.Model)
			}
			fmt.Printf("┌─ 🤖 Assistant%s [%s]\n", modelInfo, timeStr)
			printWrapped(msg.Content, "│ ", 100)
			fmt.Println("└────────────────────")
		}
		fmt.Println()
	}
}

// parseChatLine parses a JSONL line and extracts chat message if present
func parseChatLine(line string) *ChatMessage {
	var entry struct {
		Type      string          `json:"type"`
		Timestamp string          `json:"timestamp"`
		Message   json.RawMessage `json:"message"`
		// Codex fields
		Payload json.RawMessage `json:"payload"`
	}

	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return nil
	}

	ts, _ := time.Parse(time.RFC3339, entry.Timestamp)

	// Handle Claude Code format
	if entry.Type == "user" {
		var msgContent struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
		}
		if err := json.Unmarshal(entry.Message, &msgContent); err != nil {
			return nil
		}
		if msgContent.Role != "user" {
			return nil
		}
		content := extractTextContent(msgContent.Content)
		if content == "" || strings.HasPrefix(content, "<command-") {
			return nil
		}
		return &ChatMessage{
			Role:      "user",
			Content:   content,
			Timestamp: ts,
		}
	}

	if entry.Type == "assistant" {
		var msgContent struct {
			Role    string        `json:"role"`
			Model   string        `json:"model"`
			Content []interface{} `json:"content"`
		}
		if err := json.Unmarshal(entry.Message, &msgContent); err != nil {
			return nil
		}
		// Extract text and tool_use content from assistant message
		var contentParts []string
		for _, c := range msgContent.Content {
			if cMap, ok := c.(map[string]interface{}); ok {
				switch cMap["type"] {
				case "text":
					if text, ok := cMap["text"].(string); ok {
						contentParts = append(contentParts, text)
					}
				case "tool_use":
					// Format tool_use with name and input summary
					toolName, _ := cMap["name"].(string)
					if toolName != "" {
						toolStr := fmt.Sprintf("[Tool: %s]", toolName)
						// Add input details for certain tools
						if input, ok := cMap["input"].(map[string]interface{}); ok {
							switch toolName {
							case "WebSearch":
								if query, ok := input["query"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %q", toolName, query)
								}
							case "Read":
								if path, ok := input["file_path"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %s", toolName, path)
								}
							case "Write":
								if path, ok := input["file_path"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %s", toolName, path)
								}
							case "Edit":
								if path, ok := input["file_path"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %s", toolName, path)
								}
							case "Bash":
								if cmd, ok := input["command"].(string); ok {
									// Truncate long commands
									if len(cmd) > 60 {
										cmd = cmd[:57] + "..."
									}
									toolStr = fmt.Sprintf("[Tool: %s] %s", toolName, cmd)
								}
							case "Glob":
								if pattern, ok := input["pattern"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %s", toolName, pattern)
								}
							case "Grep":
								if pattern, ok := input["pattern"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %q", toolName, pattern)
								}
							case "WebFetch":
								if url, ok := input["url"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %s", toolName, url)
								}
							case "Task":
								if desc, ok := input["description"].(string); ok {
									toolStr = fmt.Sprintf("[Tool: %s] %s", toolName, desc)
								}
							}
						}
						contentParts = append(contentParts, toolStr)
					}
				}
			}
		}
		if len(contentParts) == 0 {
			return nil
		}
		return &ChatMessage{
			Role:      "assistant",
			Content:   strings.Join(contentParts, "\n"),
			Timestamp: ts,
			Model:     msgContent.Model,
		}
	}

	// Handle Codex format
	if entry.Type == "event_msg" {
		var payload struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return nil
		}
		if payload.Type == "user_message" {
			return &ChatMessage{
				Role:      "user",
				Content:   payload.Message,
				Timestamp: ts,
			}
		}
	}

	if entry.Type == "response_item" {
		var payload struct {
			Role    string `json:"role"`
			Type    string `json:"type"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(entry.Payload, &payload); err != nil {
			return nil
		}
		if payload.Role == "assistant" && payload.Type == "message" {
			return &ChatMessage{
				Role:      "assistant",
				Content:   payload.Content,
				Timestamp: ts,
			}
		}
	}

	return nil
}

// extractTextContent extracts text from content which can be string or array
func extractTextContent(content interface{}) string {
	switch c := content.(type) {
	case string:
		return c
	case []interface{}:
		var parts []string
		for _, item := range c {
			if m, ok := item.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// printWrapped prints text with wrapping and prefix
func printWrapped(text string, prefix string, maxWidth int) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if len(line) <= maxWidth {
			fmt.Printf("%s%s\n", prefix, line)
		} else {
			// Simple word wrap
			for len(line) > 0 {
				end := maxWidth
				if end > len(line) {
					end = len(line)
				}
				fmt.Printf("%s%s\n", prefix, line[:end])
				line = line[end:]
			}
		}
	}
}
