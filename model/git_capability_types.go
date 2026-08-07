package model

import "code-kanban/utils/git"

type GitCapabilityResult struct {
	Repository bool                          `json:"repository"`
	Mode       git.CapabilityMode            `json:"mode"`
	Operations git.OperationCapabilities     `json:"operations"`
	Engines    git.OperationEngines          `json:"engines"`
	Reasons    []git.CapabilityReason        `json:"reasons"`
	Worktrees  []GitWorktreeCapabilityResult `json:"worktrees"`
}

type GitWorktreeCapabilityResult struct {
	ID         string                    `json:"id"`
	Operations git.OperationCapabilities `json:"operations"`
	Engines    git.OperationEngines      `json:"engines"`
	Reasons    []git.CapabilityReason    `json:"reasons"`
}
