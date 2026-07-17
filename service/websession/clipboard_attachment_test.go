package websession

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestClipboardHTMLImageSourcesIncludesOfficeFallbacks(t *testing.T) {
	sources, err := clipboardHTMLImageSources(`
		<p>Before</p>
		<!--[if !vml]><!--><img src="file:///C:/Temp/msohtmlclip1/image001.png"><!--<![endif]-->
		<p>After</p>
	`)
	if err != nil {
		t.Fatalf("clipboardHTMLImageSources failed: %v", err)
	}
	if len(sources) != 1 || sources[0] != "file:///C:/Temp/msohtmlclip1/image001.png" {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestClipboardHTMLImageSourcesIncludesVMLWhenFallbackIsMissing(t *testing.T) {
	sources, err := clipboardHTMLImageSources(`
		<p>Before</p>
		<!--[if gte vml 1]><v:imagedata src="file:///C:/Temp/msohtmlclip1/image001.png"><![endif]-->
		<p>After</p>
	`)
	if err != nil {
		t.Fatalf("clipboardHTMLImageSources failed: %v", err)
	}
	if len(sources) != 1 || sources[0] != "file:///C:/Temp/msohtmlclip1/image001.png" {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestImportClipboardAttachmentFromHTMLReadsCurrentWordTempImage(t *testing.T) {
	wordTempDir, err := os.MkdirTemp(os.TempDir(), "msohtmlclip-test-")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(wordTempDir) })
	imagePath := filepath.Join(wordTempDir, "image001.png")
	if err := os.WriteFile(imagePath, []byte("\x89PNG\r\n\x1a\nimage-data"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	source := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(imagePath)}).String()
	store, err := newStore(t.TempDir())
	if err != nil {
		t.Fatalf("newStore failed: %v", err)
	}
	manager := &Manager{
		cfg:    Config{AttachmentSizeLimit: 1024},
		logger: zap.NewNop(),
		store:  store,
	}

	attachment, err := manager.importClipboardAttachmentFromHTML(`<img src="`+source+`">`, source)
	if err != nil {
		t.Fatalf("importClipboardAttachmentFromHTML failed: %v", err)
	}
	if attachment.Mime != "image/png" || attachment.Name != "image001.png" {
		t.Fatalf("attachment = %#v", attachment)
	}
}

func TestImportClipboardAttachmentFromHTMLRejectsUnlistedPath(t *testing.T) {
	manager := &Manager{cfg: Config{AttachmentSizeLimit: 1024}}
	source := "file:///C:/Temp/msohtmlclip1/image001.png"
	if _, err := manager.importClipboardAttachmentFromHTML(`<img src="file:///C:/Temp/msohtmlclip1/other.png">`, source); err == nil {
		t.Fatal("expected unlisted clipboard path to be rejected")
	}
}

func TestFindClipboardImagePathRejectsOrdinaryTempFileWithDiagnostic(t *testing.T) {
	ordinaryTempDir, err := os.MkdirTemp(os.TempDir(), "ordinary-clipboard-test-")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(ordinaryTempDir) })
	imagePath := filepath.Join(ordinaryTempDir, "image001.png")
	if err := os.WriteFile(imagePath, []byte("not-used"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	source := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(imagePath)}).String()

	_, err = findClipboardImagePath(`<img src="`+source+`">`, source)
	if err == nil {
		t.Fatal("expected an ordinary temporary file to be rejected")
	}
	if !strings.Contains(err.Error(), filepath.Base(imagePath)) || !strings.Contains(err.Error(), "allowed roots") {
		t.Fatalf("error does not contain path diagnostics: %v", err)
	}
}

func TestFindClipboardImagePathAcceptsWPSClipboardTempFile(t *testing.T) {
	wpsTempDir, err := os.MkdirTemp(os.TempDir(), "ksohtml-test-")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(wpsTempDir) })
	imagePath := filepath.Join(wpsTempDir, "wps11.jpg")
	if err := os.WriteFile(imagePath, []byte("not-used"), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	source := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(imagePath)}).String()

	resolvedPath, err := findClipboardImagePath(`<img src="`+source+`">`, source)
	if err != nil {
		t.Fatalf("findClipboardImagePath failed: %v", err)
	}
	wantPath, err := filepath.EvalSymlinks(imagePath)
	if err != nil {
		t.Fatalf("EvalSymlinks failed: %v", err)
	}
	if resolvedPath != wantPath {
		t.Fatalf("resolved path = %q, want %q", resolvedPath, wantPath)
	}
}
