package websession

import "testing"

func TestCodexReasoningSummary(t *testing.T) {
	tests := []struct {
		name    string
		summary any
		content any
		want    string
	}{
		{
			name:    "single title",
			summary: []string{"**Examining tests and compiler**"},
			want:    "**Examining tests and compiler**",
		},
		{
			name: "recorded separate titles",
			summary: []any{
				map[string]any{"type": "summary_text", "text": "**Checking inverse ReplaceStep handling**"},
				map[string]any{"type": "summary_text", "text": "**Refining ReplaceStep checks**"},
			},
			want: "**Checking inverse ReplaceStep handling**\n\n**Refining ReplaceStep checks**",
		},
		{
			name:    "title with body",
			summary: []any{"**Checking setup**\nInspecting the editor.", "Next step."},
			want:    "**Checking setup**\nInspecting the editor.\n\nNext step.",
		},
		{
			name: "fragments within a part",
			summary: []any{
				map[string]any{"content": []any{map[string]any{"text": "**Checking "}, map[string]any{"delta": "setup**"}}},
				"**Next step**",
			},
			want: "**Checking setup**\n\n**Next step**",
		},
		{
			name: "whitespace fragments within a part",
			summary: []any{
				map[string]any{"content": []any{"**Checking", map[string]any{"delta": " "}, "setup**"}},
			},
			want: "**Checking setup**",
		},
		{
			name:    "empty and placeholder parts",
			summary: []any{nil, " ", "<!-- -->", "**Thinking**\n<!-- -->", "**Useful title**"},
			want:    "**Useful title**",
		},
		{
			name:    "only placeholders",
			summary: []any{"<!-- -->", "**Thinking**\n<!-- -->"},
		},
		{
			name:    "literal comments remain",
			summary: []any{"Keep <!-- --> inside text.", "<!-- explanation -->", "`<!-- -->`"},
			want:    "Keep <!-- --> inside text.\n\n<!-- explanation -->\n\n`<!-- -->`",
		},
		{
			name:    "raw content excluded",
			summary: []any{"Visible summary"},
			content: []any{"Raw reasoning must not appear"},
			want:    "Visible summary",
		},
		{
			name:    "raw content without summary",
			content: []any{"Raw reasoning must not appear"},
		},
		{
			name:    "scalar summary",
			summary: " **Title** ",
			want:    "**Title**",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := map[string]any{
				"id": "reasoning-test", "type": "reasoning",
				"summary": test.summary, "content": test.content,
			}
			if got := codexToolOutput(item); got != test.want {
				t.Fatalf("live summary = %q, want %q", got, test.want)
			}
			manager := &Manager{}
			history, err := manager.mapThreadReadItem(item, 1)
			if err != nil {
				t.Fatal(err)
			}
			if history.Tool == nil || history.Tool.Output != test.want {
				t.Fatalf("history summary = %#v, want %q", history.Tool, test.want)
			}
			if history.Tool.ID != "reasoning-test" || history.Tool.Kind != "reasoning" {
				t.Fatalf("history identity changed: %#v", history.Tool)
			}
		})
	}
}
