package git

import (
	"os"
	"os/exec"
	"sync"
)

var (
	gitCommandEnv     = buildGitCommandEnv()
	testEnvOverride   []string
	testEnvOverrideMu sync.RWMutex
)

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

// SetTestEnvOverride allows tests to inject additional environment variables
// for git commands. Call with nil to clear the override.
func SetTestEnvOverride(env []string) {
	testEnvOverrideMu.Lock()
	defer testEnvOverrideMu.Unlock()
	testEnvOverride = env
}

func newGitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Env = append([]string(nil), gitCommandEnv...)

	testEnvOverrideMu.RLock()
	if len(testEnvOverride) > 0 {
		cmd.Env = append(cmd.Env, testEnvOverride...)
	}
	testEnvOverrideMu.RUnlock()

	if dir != "" {
		cmd.Dir = dir
	}
	return cmd
}
