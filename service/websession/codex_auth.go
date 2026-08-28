package websession

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	codexAuthFileName    = "auth.json"
	openAIAPIKeyEnvName  = "OPENAI_API_KEY"
	openAIBaseURLEnvName = "OPENAI_BASE_URL"
)

type codexAuthFile struct {
	OpenAIAPIKey string `json:"OPENAI_API_KEY"`
}

type codexAuthConfig struct {
	ModelProvider  string                              `toml:"model_provider"`
	ModelProviders map[string]codexModelProviderConfig `toml:"model_providers"`
}

type codexModelProviderConfig struct {
	BaseURL            string `toml:"base_url"`
	EnvKey             string `toml:"env_key"`
	RequiresOpenAIAuth bool   `toml:"requires_openai_auth"`
}

type codexAPIConnection struct {
	APIKey  string
	BaseURL string
}

type codexCredentialFileState struct {
	path    string
	exists  bool
	modTime time.Time
}

func (s codexCredentialFileState) equal(other codexCredentialFileState) bool {
	if s.path != other.path || s.exists != other.exists {
		return false
	}
	return !s.exists || s.modTime.Equal(other.modTime)
}

type codexAPIConnectionCacheEntry struct {
	loaded     bool
	configFile codexCredentialFileState
	authFile   codexCredentialFileState
	connection codexAPIConnection
}

func (e codexAPIConnectionCacheEntry) matches(configFile, authFile codexCredentialFileState) bool {
	return e.loaded && e.configFile.equal(configFile) && e.authFile.equal(authFile)
}

type codexAPIConnectionResolver struct {
	mu    sync.Mutex
	cache codexAPIConnectionCacheEntry
}

var sharedCodexAPIConnectionResolver codexAPIConnectionResolver

func currentCodexAPIConnection() codexAPIConnection {
	codexHome, err := resolveCodexHome()
	if err != nil {
		return codexAPIConnection{}
	}
	return sharedCodexAPIConnectionResolver.resolve(codexHome, os.Getenv)
}

func resolveCodexHome() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CODEX_HOME")); configured != "" {
		return filepath.Abs(configured)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".codex"), nil
}

func (r *codexAPIConnectionResolver) resolve(codexHome string, getenv func(string) string) codexAPIConnection {
	if r == nil || strings.TrimSpace(codexHome) == "" {
		return codexAPIConnection{}
	}
	if getenv == nil {
		getenv = os.Getenv
	}

	configPath := filepath.Join(codexHome, codexConfigFileName)
	authPath := filepath.Join(codexHome, codexAuthFileName)

	r.mu.Lock()
	defer r.mu.Unlock()

	configFile, configErr := inspectCodexCredentialFile(configPath)
	authFile, authErr := inspectCodexCredentialFile(authPath)
	if configErr != nil || authErr != nil {
		if r.cache.loaded && r.cache.configFile.path == configPath && r.cache.authFile.path == authPath {
			return r.cache.connection
		}
		return codexAPIConnection{}
	}
	if r.cache.matches(configFile, authFile) {
		return r.cache.connection
	}

	connection := loadCurrentCodexAPIConnection(configPath, authPath, getenv)
	configFileAfter, configErr := inspectCodexCredentialFile(configPath)
	authFileAfter, authErr := inspectCodexCredentialFile(authPath)
	if configErr == nil && authErr == nil && configFile.equal(configFileAfter) && authFile.equal(authFileAfter) {
		r.cache = codexAPIConnectionCacheEntry{
			loaded:     true,
			configFile: configFile,
			authFile:   authFile,
			connection: connection,
		}
	}
	return connection
}

func inspectCodexCredentialFile(path string) (codexCredentialFileState, error) {
	state := codexCredentialFileState{path: path}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.exists = true
	state.modTime = info.ModTime()
	return state, nil
}

func loadCurrentCodexAPIConnection(configPath, authPath string, getenv func(string) string) codexAPIConnection {
	config, ok := loadCodexAuthConfig(configPath)
	if !ok {
		return codexAPIConnection{}
	}

	providerName := strings.TrimSpace(config.ModelProvider)
	if provider, exists := config.ModelProviders[providerName]; providerName != "" && exists {
		connection := codexAPIConnection{BaseURL: strings.TrimSpace(provider.BaseURL)}
		if connection.BaseURL == "" {
			return codexAPIConnection{}
		}
		if provider.RequiresOpenAIAuth {
			connection.APIKey = loadCodexAuthFileAPIKey(authPath)
			return connection
		}
		if envKey := strings.TrimSpace(provider.EnvKey); envKey != "" {
			connection.APIKey = strings.TrimSpace(getenv(envKey))
			return connection
		}
		return codexAPIConnection{}
	}

	return codexAPIConnection{APIKey: loadCodexAuthFileAPIKey(authPath)}
}

func loadCodexAuthConfig(path string) (codexAuthConfig, bool) {
	config := codexAuthConfig{}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config, true
	}
	if err != nil || toml.Unmarshal(raw, &config) != nil {
		return codexAuthConfig{}, false
	}
	return config, true
}

func loadCodexAuthFileAPIKey(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var auth codexAuthFile
	if err := json.Unmarshal(raw, &auth); err != nil {
		return ""
	}
	return strings.TrimSpace(auth.OpenAIAPIKey)
}

func codexAppServerEnvironment() []string {
	environment := os.Environ()
	connection := currentCodexAPIConnection()
	if connection.APIKey == "" {
		return environment
	}
	environment = withEnvironmentValue(environment, openAIAPIKeyEnvName, connection.APIKey)
	if connection.BaseURL != "" {
		environment = withEnvironmentValue(environment, openAIBaseURLEnvName, connection.BaseURL)
	}
	return environment
}

func withEnvironmentValue(environment []string, name, value string) []string {
	updated := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		separator := strings.IndexByte(entry, '=')
		if separator > 0 && environmentNamesEqual(entry[:separator], name) {
			continue
		}
		updated = append(updated, entry)
	}
	return append(updated, name+"="+value)
}

func environmentNamesEqual(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
