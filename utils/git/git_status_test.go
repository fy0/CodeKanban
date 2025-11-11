package git

import "testing"

func TestParseGitStatusOutput(t *testing.T) {
	output := `
# branch.oid 3d2b07e3ce0b
# branch.head main
# branch.upstream origin/main
# branch.ab +2 -1
1 M. N... 0000000 0000000 0000000 README.md
1 .M N... 0000000 0000000 0000000 file.go
1 MM N... 0000000 0000000 0000000 both.txt
? new.txt
u UU N... 0000000 0000000 0000000 conflict.txt
`

	status := parseGitStatusOutput(output)
	if status == nil {
		t.Fatalf("parseGitStatusOutput returned nil")
	}

	if status.Branch != "main" {
		t.Fatalf("expected branch main got %q", status.Branch)
	}
	if status.Ahead != 2 || status.Behind != 1 {
		t.Fatalf("unexpected ahead/behind counts: %+v", status)
	}
	if status.Staged != 2 {
		t.Fatalf("expected staged=2 got %d", status.Staged)
	}
	if status.Modified != 2 {
		t.Fatalf("expected modified=2 got %d", status.Modified)
	}
	if status.Untracked != 1 {
		t.Fatalf("expected untracked=1 got %d", status.Untracked)
	}
	if status.Conflicted != 1 {
		t.Fatalf("expected conflicted=1 got %d", status.Conflicted)
	}
}
