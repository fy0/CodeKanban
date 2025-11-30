package terminal

import (
	"sync"
	"time"

	"code-kanban/utils/ai_assistant2"
)

// CompletionRecord 代表一个AI执行完成的记录
type CompletionRecord struct {
	ID          string                         `json:"id"`
	SessionID   string                         `json:"sessionId"`
	ProjectID   string                         `json:"projectId"`
	ProjectName string                         `json:"projectName,omitempty"`
	Title       string                         `json:"title"`
	Assistant   *ai_assistant2.AIAssistantInfo `json:"assistant"`
	CompletedAt time.Time                      `json:"completedAt"`
	// State 表示当前卡片状态，working 时仍保留卡片
	State string `json:"state,omitempty"`
	// LastUserInput 存储用户上次输入的信息
	LastUserInput string `json:"lastUserInput,omitempty"`
	// Dismissed 标记用户是否已主动关闭此通知
	Dismissed bool `json:"dismissed"`
}

// ApprovalRecord 代表一个等待审批的记录
type ApprovalRecord struct {
	ID          string                         `json:"id"`
	SessionID   string                         `json:"sessionId"`
	ProjectID   string                         `json:"projectId"`
	ProjectName string                         `json:"projectName,omitempty"`
	Title       string                         `json:"title"`
	Assistant   *ai_assistant2.AIAssistantInfo `json:"assistant"`
	RequestedAt time.Time                      `json:"requestedAt"`
	// Dismissed 标记用户是否已主动关闭此通知
	Dismissed bool `json:"dismissed"`
}

// RecordManager 管理完成记录和审批记录
type RecordManager struct {
	mu sync.RWMutex
	// completions 存储完成记录，key 为记录ID
	completions map[string]*CompletionRecord
	// approvals 存储审批记录，key 为记录ID
	approvals map[string]*ApprovalRecord
	// sessionCompletions 按 sessionId 索引，用于快速查找和清理
	sessionCompletions map[string][]string // sessionId -> []recordId
	// sessionApprovals 按 sessionId 索引
	sessionApprovals map[string][]string // sessionId -> []recordId
}

// NewRecordManager 创建新的记录管理器
func NewRecordManager() *RecordManager {
	return &RecordManager{
		completions:        make(map[string]*CompletionRecord),
		approvals:          make(map[string]*ApprovalRecord),
		sessionCompletions: make(map[string][]string),
		sessionApprovals:   make(map[string][]string),
	}
}

// AddCompletion 添加一个完成记录
func (rm *RecordManager) AddCompletion(record *CompletionRecord) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if record.State == "" {
		record.State = "completed"
	}
	rm.completions[record.ID] = record
	rm.sessionCompletions[record.SessionID] = append(rm.sessionCompletions[record.SessionID], record.ID)
}

// AddApproval 添加一个审批记录
func (rm *RecordManager) AddApproval(record *ApprovalRecord) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.approvals[record.ID] = record
	rm.sessionApprovals[record.SessionID] = append(rm.sessionApprovals[record.SessionID], record.ID)
}

// GetCompletions 获取所有未关闭的完成记录
func (rm *RecordManager) GetCompletions() []*CompletionRecord {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*CompletionRecord, 0)
	for _, record := range rm.completions {
		if !record.Dismissed {
			result = append(result, record)
		}
	}
	return result
}

// GetApprovals 获取所有未关闭的审批记录
func (rm *RecordManager) GetApprovals() []*ApprovalRecord {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	result := make([]*ApprovalRecord, 0)
	for _, record := range rm.approvals {
		if !record.Dismissed {
			result = append(result, record)
		}
	}
	return result
}

// DismissCompletion 关闭一个完成记录
func (rm *RecordManager) DismissCompletion(recordID string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if record, exists := rm.completions[recordID]; exists {
		record.Dismissed = true
		return true
	}
	return false
}

// DismissApproval 关闭一个审批记录
func (rm *RecordManager) DismissApproval(recordID string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if record, exists := rm.approvals[recordID]; exists {
		record.Dismissed = true
		return true
	}
	return false
}

// ClearSessionRecords 清除某个 session 的所有记录（当 session 关闭或状态变化时）
func (rm *RecordManager) ClearSessionRecords(sessionID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.clearCompletionsLocked(sessionID)
	rm.clearApprovalsLocked(sessionID)
}

// ClearCompletionsBySession 清除某个 session 的所有完成记录
func (rm *RecordManager) ClearCompletionsBySession(sessionID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.clearCompletionsLocked(sessionID)
}

// ClearApprovalsBySession 清除某个 session 的所有审批记录（当状态从 waiting_approval 变化时）
func (rm *RecordManager) ClearApprovalsBySession(sessionID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.clearApprovalsLocked(sessionID)
}

// UpdateCompletionStateBySession 更新 session 对应的完成记录状态（例如切回 working）
func (rm *RecordManager) UpdateCompletionStateBySession(sessionID string, state string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	updated := false
	if recordIDs, exists := rm.sessionCompletions[sessionID]; exists {
		for _, recordID := range recordIDs {
			if record, ok := rm.completions[recordID]; ok {
				record.State = state
				updated = true
			}
		}
	}
	return updated
}

// UpdateCompletionBySession 更新 session 对应的完成记录状态和用户输入
// 如果 userInput 非空，则同时更新 LastUserInput 字段
func (rm *RecordManager) UpdateCompletionBySession(sessionID string, state string, userInput string) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	updated := false
	if recordIDs, exists := rm.sessionCompletions[sessionID]; exists {
		for _, recordID := range recordIDs {
			if record, ok := rm.completions[recordID]; ok {
				record.State = state
				// 只有当有新的用户输入时才更新
				if userInput != "" {
					record.LastUserInput = userInput
				}
				updated = true
			}
		}
	}
	return updated
}

// GetCompletion 获取单个完成记录
func (rm *RecordManager) GetCompletion(recordID string) *CompletionRecord {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.completions[recordID]
}

// GetApproval 获取单个审批记录
func (rm *RecordManager) GetApproval(recordID string) *ApprovalRecord {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.approvals[recordID]
}

func (rm *RecordManager) clearCompletionsLocked(sessionID string) {
	if recordIDs, exists := rm.sessionCompletions[sessionID]; exists {
		for _, recordID := range recordIDs {
			delete(rm.completions, recordID)
		}
		delete(rm.sessionCompletions, sessionID)
	}
}

func (rm *RecordManager) clearApprovalsLocked(sessionID string) {
	if recordIDs, exists := rm.sessionApprovals[sessionID]; exists {
		for _, recordID := range recordIDs {
			delete(rm.approvals, recordID)
		}
		delete(rm.sessionApprovals, sessionID)
	}
}
