package websession

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/net/html"
)

var (
	ErrLocalClipboardUnavailable = errors.New("local clipboard access is unavailable")
	officeDownlevelFallback      = regexp.MustCompile(`(?is)<!--\s*\[if\s+!vml\]\s*><!-->(.*?)<!--<!\s*\[endif\]\s*-->`)
	officeHiddenFallback         = regexp.MustCompile(`(?is)<!--\s*\[if\s+!vml\]\s*>(.*?)<!\s*\[endif\]\s*-->`)
	officeVMLContent             = regexp.MustCompile(`(?is)<!--\s*\[if[^\]]*\bvml\b[^\]]*\]\s*>(.*?)<!\s*\[endif\]\s*-->`)
)

func exposeOfficeClipboardFallbacks(value string) string {
	hasNonVMLFallback := officeDownlevelFallback.MatchString(value) || officeHiddenFallback.MatchString(value)
	value = officeDownlevelFallback.ReplaceAllString(value, "$1")
	value = officeHiddenFallback.ReplaceAllString(value, "$1")
	if !hasNonVMLFallback {
		value = officeVMLContent.ReplaceAllString(value, "$1")
	}
	return value
}

func clipboardHTMLImageSources(value string) ([]string, error) {
	document, err := html.Parse(strings.NewReader(exposeOfficeClipboardFallbacks(value)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse clipboard HTML: %w", err)
	}

	sources := make([]string, 0)
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			tagName := strings.ToLower(strings.TrimSpace(node.Data))
			if tagName == "img" || tagName == "imagedata" || tagName == "v:imagedata" {
				for _, attributeName := range []string{"src", "data-src", "data-original"} {
					for _, attribute := range node.Attr {
						if strings.EqualFold(attribute.Key, attributeName) {
							source := strings.TrimSpace(attribute.Val)
							if source != "" {
								sources = append(sources, source)
							}
							return
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return sources, nil
}

func localPathFromClipboardFileURL(source string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || !strings.EqualFold(parsed.Scheme, "file") {
		return "", fmt.Errorf("clipboard image is not a local file URL")
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		return "", fmt.Errorf("network file URLs are not supported")
	}

	decodedPath, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		return "", fmt.Errorf("invalid clipboard image path: %w", err)
	}
	if len(decodedPath) >= 3 && decodedPath[0] == '/' && decodedPath[2] == ':' {
		decodedPath = decodedPath[1:]
	}
	filePath := filepath.Clean(filepath.FromSlash(decodedPath))
	if !filepath.IsAbs(filePath) {
		return "", fmt.Errorf("clipboard image path is not absolute")
	}
	return filePath, nil
}

type wordClipboardRoot struct {
	path                      string
	requiredDirectoryPrefixes []string
}

var officeClipboardTempDirectoryPrefixes = []string{"msohtmlclip", "ksohtml"}

func wordClipboardRoots() []wordClipboardRoot {
	roots := []wordClipboardRoot{{
		path:                      os.TempDir(),
		requiredDirectoryPrefixes: officeClipboardTempDirectoryPrefixes,
	}}
	if userCacheDir, err := os.UserCacheDir(); err == nil {
		roots = append(roots,
			wordClipboardRoot{
				path:                      filepath.Join(userCacheDir, "Temp"),
				requiredDirectoryPrefixes: officeClipboardTempDirectoryPrefixes,
			},
			wordClipboardRoot{
				path: filepath.Join(userCacheDir, "Microsoft", "Windows", "INetCache", "Content.Word"),
			},
		)
	}
	return roots
}

func pathHasDirectoryPrefix(relativePath string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	for _, part := range strings.Split(relativePath, string(filepath.Separator)) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(strings.ToLower(part), strings.ToLower(prefix)) {
				return true
			}
		}
	}
	return false
}

func resolveWordClipboardTempPath(filePath string) (string, bool) {
	absolutePath, err := filepath.Abs(filePath)
	if err != nil {
		return "", false
	}
	resolvedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", false
	}
	for _, root := range wordClipboardRoots() {
		resolvedRoot, rootErr := filepath.EvalSymlinks(root.path)
		if rootErr != nil {
			continue
		}
		relativePath, relativeErr := filepath.Rel(resolvedRoot, resolvedPath)
		if relativeErr != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			continue
		}
		if pathHasDirectoryPrefix(relativePath, root.requiredDirectoryPrefixes) {
			return resolvedPath, true
		}
	}
	return "", false
}

func wordClipboardRootPaths() []string {
	paths := make([]string, 0, len(wordClipboardRoots()))
	seen := make(map[string]struct{})
	for _, root := range wordClipboardRoots() {
		cleaned := filepath.Clean(root.path)
		key := strings.ToLower(cleaned)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		paths = append(paths, cleaned)
	}
	return paths
}

func findClipboardImagePath(clipboardHTML, requestedSource string) (string, error) {
	requestedPath, err := localPathFromClipboardFileURL(requestedSource)
	if err != nil {
		return "", err
	}
	sources, err := clipboardHTMLImageSources(clipboardHTML)
	if err != nil {
		return "", err
	}
	for _, source := range sources {
		candidatePath, candidateErr := localPathFromClipboardFileURL(source)
		if candidateErr == nil && strings.EqualFold(candidatePath, requestedPath) {
			resolvedPath, ok := resolveWordClipboardTempPath(candidatePath)
			if !ok {
				return "", fmt.Errorf(
					"clipboard image is outside the allowed Office temporary directories (path: %q; allowed roots: %s)",
					candidatePath,
					strings.Join(wordClipboardRootPaths(), ", "),
				)
			}
			return resolvedPath, nil
		}
	}
	return "", fmt.Errorf("clipboard image is no longer present in the current clipboard")
}

func (m *Manager) importClipboardAttachmentFromHTML(clipboardHTML, source string) (Attachment, error) {
	filePath, err := findClipboardImagePath(clipboardHTML, source)
	if err != nil {
		return Attachment{}, err
	}
	file, err := os.Open(filePath)
	if err != nil {
		return Attachment{}, fmt.Errorf("failed to open Office clipboard image: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, m.cfg.AttachmentSizeLimit+1))
	if err != nil {
		return Attachment{}, fmt.Errorf("failed to read Office clipboard image: %w", err)
	}
	if int64(len(data)) > m.cfg.AttachmentSizeLimit {
		return Attachment{}, fmt.Errorf("attachment too large")
	}
	mimeType, err := remoteAttachmentMimeType("", data)
	if err != nil {
		return Attachment{}, fmt.Errorf("Office clipboard file is not an image")
	}
	return m.saveAttachmentBytes(filepath.Base(filePath), mimeType, data)
}

func (m *Manager) ImportLocalClipboardAttachment(ctx context.Context, source string) (Attachment, error) {
	clipboardHTML, err := readLocalClipboardHTML(ctx)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("failed to read local clipboard HTML", zap.Error(err))
		}
		return Attachment{}, err
	}
	attachment, err := m.importClipboardAttachmentFromHTML(clipboardHTML, source)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn(
				"failed to import Office clipboard image",
				zap.String("source", source),
				zap.Strings("allowedRoots", wordClipboardRootPaths()),
				zap.Error(err),
			)
		}
		return Attachment{}, err
	}
	return attachment, nil
}
