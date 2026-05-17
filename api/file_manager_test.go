package api

import "testing"

func TestMimeTypeForDownloadTreatsModAsText(t *testing.T) {
	t.Parallel()

	if got := mimeTypeForDownload("go.mod"); got != "text/plain" {
		t.Fatalf("mimeTypeForDownload(go.mod) = %q, want text/plain", got)
	}
}
