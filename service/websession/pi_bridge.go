package websession

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	piBridgeCommandName = "codekanban-navigate"
	piBridgeMarkerType  = "codekanban.active-leaf.v1"
)

//go:embed pi_bridge/extension.ts
var piBridgeSource []byte

var piBridgeMaterializeMu sync.Mutex

func (m *Manager) materializePiBridge() (string, error) {
	piBridgeMaterializeMu.Lock()
	defer piBridgeMaterializeMu.Unlock()
	if m == nil || strings.TrimSpace(m.cfg.DataDir) == "" {
		return "", errors.New("Pi bridge data directory is not configured")
	}
	digest := sha256.Sum256(piBridgeSource)
	hash := hex.EncodeToString(digest[:])
	root, err := filepath.Abs(filepath.Join(m.cfg.DataDir, "pi-bridge"))
	if err != nil {
		return "", fmt.Errorf("resolve Pi bridge directory: %w", err)
	}
	dir := filepath.Join(root, hash)
	path := filepath.Join(dir, "extension.ts")

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create Pi bridge directory: %w", err)
	}
	if err := ensurePiBridgeContained(root, path); err != nil {
		return "", err
	}
	if _, err := validatePiBridgeArtifact(path); err == nil {
		return filepath.Clean(path), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect Pi bridge artifact: %w", err)
	}

	temp, err := os.CreateTemp(dir, ".extension-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create Pi bridge artifact: %w", err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return "", fmt.Errorf("secure Pi bridge artifact: %w", err)
	}
	if _, err := temp.Write(piBridgeSource); err != nil {
		return "", fmt.Errorf("write Pi bridge artifact: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return "", fmt.Errorf("sync Pi bridge artifact: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close Pi bridge artifact: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		// Another runtime may have installed this immutable hash artifact first.
		// Accept only the exact embedded regular file; every other race fails closed.
		if _, validateErr := validatePiBridgeArtifact(path); validateErr != nil {
			return "", fmt.Errorf("install Pi bridge artifact: %w", err)
		}
		return filepath.Clean(path), nil
	}
	removeTemp = false
	return filepath.Clean(path), nil
}

func validatePiBridgeArtifact(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("Pi bridge artifact is not a regular file")
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Pi bridge artifact: %w", err)
	}
	if !bytes.Equal(onDisk, piBridgeSource) {
		return nil, errors.New("Pi bridge artifact content does not match the embedded extension")
	}
	return info, nil
}

func ensurePiBridgeContained(root, path string) error {
	canonicalRoot, err := canonicalPiRuntimePath(root)
	if err != nil {
		return fmt.Errorf("resolve Pi bridge root: %w", err)
	}
	canonicalPath, err := canonicalPiRuntimePath(path)
	if err != nil {
		return fmt.Errorf("resolve Pi bridge artifact: %w", err)
	}
	relative, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil || relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("Pi bridge artifact is outside the managed bridge root")
	}
	return nil
}

type piRPCSourceInfo struct {
	Path    string `json:"path"`
	Source  string `json:"source"`
	Scope   string `json:"scope"`
	Origin  string `json:"origin"`
	BaseDir string `json:"baseDir"`
}

type piRPCSlashCommand struct {
	Name       string          `json:"name"`
	Source     string          `json:"source"`
	SourceInfo piRPCSourceInfo `json:"sourceInfo"`
}

func validatePiBridgeCommands(commands []piRPCSlashCommand, expectedPath string) error {
	matches := make([]piRPCSlashCommand, 0, 1)
	for _, command := range commands {
		if strings.TrimSpace(command.Name) == piBridgeCommandName {
			matches = append(matches, command)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf("Pi bridge command provenance is ambiguous: found %d registrations", len(matches))
	}
	command := matches[0]
	if strings.TrimSpace(command.Source) != "extension" ||
		strings.TrimSpace(command.SourceInfo.Source) != "cli" ||
		strings.TrimSpace(command.SourceInfo.Scope) != "temporary" ||
		strings.TrimSpace(command.SourceInfo.Origin) != "top-level" ||
		!samePiRuntimePath(command.SourceInfo.Path, expectedPath) {
		return errors.New("Pi bridge command provenance validation failed")
	}
	return nil
}
