package model

import "code-kanban/utils/git"

// BranchListResult describes the response for listing branches.
type BranchListResult struct {
	Local  []git.BranchInfo `json:"local"`
	Remote []git.BranchInfo `json:"remote"`
}

// MergeResult captures merge command outcomes.
type MergeResult struct {
	Success   bool     `json:"success"`
	Conflicts []string `json:"conflicts"`
	Message   string   `json:"message"`
}

// MergeBranchOptions describes optional behaviors for merge operations.
type MergeBranchOptions struct {
	TargetBranch  string
	Strategy      string
	Commit        bool
	CommitMessage string
}
