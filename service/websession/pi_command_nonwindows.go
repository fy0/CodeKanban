//go:build !windows

package websession

import (
	"context"
	"os/exec"
)

func buildWindowsBatchCommand(
	ctx context.Context,
	comspec string,
	batchPath string,
	args []string,
) *exec.Cmd {
	return exec.CommandContext(ctx, batchPath, args...)
}
