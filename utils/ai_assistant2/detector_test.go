package ai_assistant2

import (
	"testing"

	"code-kanban/utils/ai_assistant2/types"
)

func TestDetectFromCommand(t *testing.T) {
	t.Parallel()

	detector := NewAssistantDetector()

	tests := []struct {
		name    string
		command string
		want    types.AssistantType
	}{
		{
			name:    "codex package path",
			command: "node /workspace/node_modules/@openai/codex/bin/codex.js",
			want:    types.AssistantTypeCodex,
		},
		{
			name:    "codex direct executable path",
			command: "/usr/local/bin/codex --model gpt-5",
			want:    types.AssistantTypeCodex,
		},
		{
			name:    "codex node wrapped executable",
			command: "node /usr/local/bin/codex 1",
			want:    types.AssistantTypeCodex,
		},
		{
			name:    "codex shell wrapped executable",
			command: `bash -lc "codex --model gpt-5"`,
			want:    types.AssistantTypeCodex,
		},
		{
			name:    "pi direct executable path",
			command: "/usr/local/bin/pi --mode rpc",
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi npm package path",
			command: "node /usr/local/lib/node_modules/@earendil-works/pi-coding-agent/dist/cli.js --mode rpc",
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi legacy npm package path",
			command: "node /usr/local/lib/node_modules/@mariozechner/pi-coding-agent/dist/cli.js",
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi node wrapped executable",
			command: "node /usr/local/bin/pi --version",
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi node package paths containing spaces",
			command: `"C:\Program Files\nodejs\node.exe" "C:\Users\Dev User\AppData\Roaming\npm\node_modules\@earendil-works\pi-coding-agent\dist\cli.js" --mode rpc`,
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi direct Windows shim path containing spaces",
			command: `"C:\Dev Tools\pi.cmd" --mode rpc`,
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi bash wrapped executable",
			command: `bash -lc "pi --model openai/gpt-5"`,
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi cmd wrapped npm shim",
			command: `cmd.exe /d /s /c "C:\\tools\\pi.cmd --mode rpc"`,
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi cmd canonical outer quote wrapper",
			command: `cmd.exe /d /s /c ""C:\Dev Tools\pi.cmd" "--mode" "rpc""`,
			want:    types.AssistantTypePi,
		},
		{
			name:    "pi powershell wrapped npm shim",
			command: `pwsh.exe -Command "& C:\\tools\\pi.ps1 --version"`,
			want:    types.AssistantTypePi,
		},
		{
			name:    "claude direct executable path",
			command: "/usr/local/bin/claude --print",
			want:    types.AssistantTypeClaudeCode,
		},
		{
			name:    "claude code direct executable path",
			command: "/usr/local/bin/claude-code --print",
			want:    types.AssistantTypeClaudeCode,
		},
		{
			name:    "claude node wrapped executable",
			command: "node /usr/local/bin/claude 1",
			want:    types.AssistantTypeClaudeCode,
		},
		{
			name:    "claude shell wrapped executable",
			command: `bash -lc "claude --dangerously-skip-permissions"`,
			want:    types.AssistantTypeClaudeCode,
		},
		{
			name:    "claude code router direct command",
			command: "ccr code --model sonnet",
			want:    types.AssistantTypeClaudeCode,
		},
		{
			name:    "claude code router node cli command",
			command: "node /usr/local/lib/node_modules/@musistudio/claude-code-router/dist/cli.js code --model sonnet",
			want:    types.AssistantTypeClaudeCode,
		},
		{
			name:    "claude code router status is not assistant",
			command: "ccr status",
			want:    types.AssistantTypeUnknown,
		},
		{
			name:    "regular shell command is not ai assistant",
			command: `bash -lc "echo codex"`,
			want:    types.AssistantTypeUnknown,
		},
		{
			name:    "regular node command is not ai assistant",
			command: "node /usr/local/bin/serve 3000",
			want:    types.AssistantTypeUnknown,
		},
		{
			name:    "pi as a regular argument is not assistant",
			command: `bash -lc "echo pi"`,
			want:    types.AssistantTypeUnknown,
		},
		{
			name:    "pi package path as regular text is not assistant",
			command: `bash -lc "echo @earendil-works/pi-coding-agent/dist/cli.js"`,
			want:    types.AssistantTypeUnknown,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			info := detector.DetectFromCommand(tt.command)
			if tt.want == types.AssistantTypeUnknown {
				if info != nil {
					t.Fatalf("expected nil detection, got %+v", info)
				}
				return
			}

			if info == nil {
				t.Fatalf("expected detection for %q", tt.command)
			}
			if info.Type != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, info.Type)
			}
		})
	}
}
