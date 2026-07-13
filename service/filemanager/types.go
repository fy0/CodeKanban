package filemanager

import "time"

type ScopeKind string

const (
	ScopeKindProject  ScopeKind = "project"
	ScopeKindWorktree ScopeKind = "worktree"
)

type EntryKind string

const (
	EntryKindFile      EntryKind = "file"
	EntryKindDirectory EntryKind = "directory"
	EntryKindSymlink   EntryKind = "symlink"
)

type PreviewKind string

const (
	PreviewKindImage    PreviewKind = "image"
	PreviewKindText     PreviewKind = "text"
	PreviewKindMarkdown PreviewKind = "markdown"
	PreviewKindPDF      PreviewKind = "pdf"
	PreviewKindAudio    PreviewKind = "audio"
	PreviewKindVideo    PreviewKind = "video"
	PreviewKindBinary   PreviewKind = "binary"
)

type GitStatusKind string

const (
	GitStatusKindModified   GitStatusKind = "modified"
	GitStatusKindAdded      GitStatusKind = "added"
	GitStatusKindDeleted    GitStatusKind = "deleted"
	GitStatusKindRenamed    GitStatusKind = "renamed"
	GitStatusKindUntracked  GitStatusKind = "untracked"
	GitStatusKindConflicted GitStatusKind = "conflicted"
	GitStatusKindDirty      GitStatusKind = "dirty"
)

type GitStatus struct {
	Kind         GitStatusKind `json:"kind"`
	PreviousPath string        `json:"previousPath,omitempty"`
}

type Scope struct {
	ID         string    `json:"id"`
	Kind       ScopeKind `json:"kind"`
	Label      string    `json:"label"`
	RootPath   string    `json:"rootPath"`
	WorktreeID string    `json:"worktreeId,omitempty"`
}

type Breadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Entry struct {
	Name        string      `json:"name"`
	Path        string      `json:"path"`
	Kind        EntryKind   `json:"kind"`
	Size        int64       `json:"size"`
	ModifiedAt  time.Time   `json:"modifiedAt"`
	Mime        string      `json:"mime,omitempty"`
	Extension   string      `json:"extension,omitempty"`
	PreviewKind PreviewKind `json:"previewKind"`
	Hidden      bool        `json:"hidden"`
	GitStatus   *GitStatus  `json:"gitStatus,omitempty"`
}

type ListResult struct {
	Scope       Scope        `json:"scope"`
	CurrentPath string       `json:"currentPath"`
	ParentPath  string       `json:"parentPath,omitempty"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
	Entries     []Entry      `json:"entries"`
}

type SearchResult struct {
	Scope       Scope   `json:"scope"`
	CurrentPath string  `json:"currentPath"`
	Entries     []Entry `json:"entries"`
	Truncated   bool    `json:"truncated"`
}

type PreviewResult struct {
	Entry       Entry       `json:"entry"`
	PreviewKind PreviewKind `json:"previewKind"`
	TextContent string      `json:"textContent,omitempty"`
	Truncated   bool        `json:"truncated"`
}

type ChangeEntry struct {
	Name           string      `json:"name"`
	Path           string      `json:"path"`
	PreviewKind    PreviewKind `json:"previewKind"`
	Hidden         bool        `json:"hidden"`
	Exists         bool        `json:"exists"`
	Status         GitStatus   `json:"status"`
	Additions      int64       `json:"additions"`
	Deletions      int64       `json:"deletions"`
	StatsAvailable bool        `json:"statsAvailable"`
}

type ChangesResult struct {
	Scope             Scope         `json:"scope"`
	Entries           []ChangeEntry `json:"entries"`
	ChangeToken       string        `json:"changeToken"`
	Truncated         bool          `json:"truncated"`
	StatsComplete     bool          `json:"statsComplete"`
	StatsTimedOut     bool          `json:"statsTimedOut"`
	UntrackedIncluded bool          `json:"untrackedIncluded"`
	WarningReason     string        `json:"warningReason,omitempty"`
}

type ListChangesOptions struct {
	IncludeUntracked *bool
	WithStats        *bool
	Timeout          time.Duration
	MaxEntries       int
}

type ChangesSummaryOptions struct {
	IncludeUntracked bool
	WithStats        bool
	StatsTimeout     time.Duration
}

type ChangesSummaryResult struct {
	Scope         Scope  `json:"scope"`
	Count         int64  `json:"count"`
	ChangeToken   string `json:"changeToken"`
	Additions     *int64 `json:"additions"`
	Deletions     *int64 `json:"deletions"`
	StatsComplete bool   `json:"statsComplete"`
	StatsTimedOut bool   `json:"statsTimedOut"`
}

type DiffResult struct {
	Path         string     `json:"path"`
	Status       *GitStatus `json:"status,omitempty"`
	Available    bool       `json:"available"`
	Reason       string     `json:"reason,omitempty"`
	PreviousPath string     `json:"previousPath,omitempty"`
	DiffText     string     `json:"diffText,omitempty"`
	ComparedTo   string     `json:"comparedTo"`
}

type FileRef struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type BulkFailure struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Message string `json:"message"`
}

type BulkResult struct {
	Succeeded []FileRef     `json:"succeeded"`
	Failed    []BulkFailure `json:"failed"`
}

type ArchiveJob struct {
	ID        string    `json:"archiveId"`
	FileName  string    `json:"fileName"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type UploadSession struct {
	ID         string    `json:"uploadId"`
	ProjectID  string    `json:"projectId"`
	ScopeID    string    `json:"scopeId"`
	Directory  string    `json:"directoryPath"`
	TargetPath string    `json:"targetPath"`
	FileName   string    `json:"fileName"`
	Size       int64     `json:"size"`
	Offset     int64     `json:"offset"`
	ChunkSize  int64     `json:"chunkSize"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
}
