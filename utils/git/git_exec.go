package git

import (
	"os"
	"os/exec"
)

var gitCommandEnv = buildGitCommandEnv()

func buildGitCommandEnv() []string {
	env := os.Environ()
	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
	)
	return env
}

func newGitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = append([]string(nil), gitCommandEnv...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}
