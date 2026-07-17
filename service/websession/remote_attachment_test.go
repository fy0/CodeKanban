package websession

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap"
)

type remoteAttachmentRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn remoteAttachmentRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newRemoteAttachmentTestManager(t *testing.T, body string, contentType string) *Manager {
	t.Helper()
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore failed: %v", err)
	}
	client := &http.Client{Transport: remoteAttachmentRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{contentType}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}
	return &Manager{
		cfg: Config{
			AttachmentSizeLimit:    1024,
			RemoteAttachmentClient: client,
		},
		logger: zap.NewNop(),
		store:  store,
	}
}

func TestImportRemoteAttachmentSavesDownloadedImage(t *testing.T) {
	manager := newRemoteAttachmentTestManager(t, "\x89PNG\r\n\x1a\nimage-data", "image/png")

	attachment, err := manager.ImportRemoteAttachment(context.Background(), "https://images.example/report.png")
	if err != nil {
		t.Fatalf("ImportRemoteAttachment failed: %v", err)
	}
	if attachment.Name != "report.png" {
		t.Fatalf("name = %q, want report.png", attachment.Name)
	}
	if attachment.Mime != "image/png" {
		t.Fatalf("mime = %q, want image/png", attachment.Mime)
	}
}

func TestImportRemoteAttachmentRejectsNonImage(t *testing.T) {
	manager := newRemoteAttachmentTestManager(t, "<html>not an image</html>", "text/html")

	if _, err := manager.ImportRemoteAttachment(context.Background(), "https://images.example/report.png"); err == nil {
		t.Fatal("expected non-image response to be rejected")
	}
}

func TestImportRemoteAttachmentRejectsOversizedImage(t *testing.T) {
	manager := newRemoteAttachmentTestManager(t, "\x89PNG\r\n\x1a\n"+strings.Repeat("x", 2048), "image/png")

	if _, err := manager.ImportRemoteAttachment(context.Background(), "https://images.example/report.png"); err == nil {
		t.Fatal("expected oversized image to be rejected")
	}
}

func TestRemoteAttachmentURLAndAddressRestrictions(t *testing.T) {
	for _, rawURL := range []string{
		"file:///tmp/image.png",
		"http://user:password@example.com/image.png",
		"http://127.0.0.1/image.png",
	} {
		if _, err := validateRemoteAttachmentURL(rawURL); err == nil {
			t.Fatalf("expected %q to be rejected", rawURL)
		}
	}

	if !isAllowedRemoteAttachmentIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("expected public IP to be allowed")
	}
	for _, blocked := range []string{"10.0.0.1", "100.64.0.1", "169.254.169.254", "::1"} {
		if isAllowedRemoteAttachmentIP(net.ParseIP(blocked)) {
			t.Fatalf("expected %s to be blocked", blocked)
		}
	}
}
