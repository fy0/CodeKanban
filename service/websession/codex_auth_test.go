package websession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexAPIConnectionResolverCachesUntilAuthFileModificationTimeChanges(t *testing.T) {
	codexHome := t.TempDir()
	configPath := filepath.Join(codexHome, codexConfigFileName)
	authPath := filepath.Join(codexHome, codexAuthFileName)
	writeCodexTestFile(t, configPath, []byte(`
model_provider = "custom"

[model_providers.custom]
base_url = "https://custom.example/v1"
requires_openai_auth = true
`))
	writeCodexTestAuthFile(t, authPath, "key-one")
	configModTime := codexTestFileModTime(t, configPath)
	authModTime := codexTestFileModTime(t, authPath)

	var resolver codexAPIConnectionResolver
	requireCodexTestConnection(t, resolver.resolve(codexHome, nil), "key-one", "https://custom.example/v1")

	writeCodexTestAuthFile(t, authPath, "key-two")
	setCodexTestFileModTime(t, authPath, authModTime)
	setCodexTestFileModTime(t, configPath, configModTime)
	requireCodexTestConnection(t, resolver.resolve(codexHome, nil), "key-one", "https://custom.example/v1")

	setCodexTestFileModTime(t, authPath, authModTime.Add(2*time.Second))
	requireCodexTestConnection(t, resolver.resolve(codexHome, nil), "key-two", "https://custom.example/v1")
}

func TestCodexAPIConnectionResolverCachesUntilConfigFileModificationTimeChanges(t *testing.T) {
	codexHome := t.TempDir()
	configPath := filepath.Join(codexHome, codexConfigFileName)
	authPath := filepath.Join(codexHome, codexAuthFileName)
	writeCodexTestFile(t, configPath, []byte(`
model_provider = "first"

[model_providers.first]
base_url = "https://first.example/v1"
env_key = "FIRST_API_KEY"
`))
	writeCodexTestAuthFile(t, authPath, "auth-key")
	configModTime := codexTestFileModTime(t, configPath)
	authModTime := codexTestFileModTime(t, authPath)

	values := map[string]string{
		"FIRST_API_KEY":  "first-key",
		"SECOND_API_KEY": "second-key",
	}
	getenv := func(name string) string { return values[name] }
	var resolver codexAPIConnectionResolver
	requireCodexTestConnection(t, resolver.resolve(codexHome, getenv), "first-key", "https://first.example/v1")

	values["FIRST_API_KEY"] = "changed-without-file-update"
	requireCodexTestConnection(t, resolver.resolve(codexHome, getenv), "first-key", "https://first.example/v1")

	writeCodexTestFile(t, configPath, []byte(`
model_provider = "second"

[model_providers.second]
base_url = "https://second.example/v1"
env_key = "SECOND_API_KEY"
`))
	setCodexTestFileModTime(t, configPath, configModTime)
	setCodexTestFileModTime(t, authPath, authModTime)
	requireCodexTestConnection(t, resolver.resolve(codexHome, getenv), "first-key", "https://first.example/v1")

	setCodexTestFileModTime(t, configPath, configModTime.Add(2*time.Second))
	requireCodexTestConnection(t, resolver.resolve(codexHome, getenv), "second-key", "https://second.example/v1")
}

func TestCodexAPIConnectionResolverUsesOpenAIAuthBeforeProviderEnvKey(t *testing.T) {
	codexHome := t.TempDir()
	writeCodexTestFile(t, filepath.Join(codexHome, codexConfigFileName), []byte(`
model_provider = "custom"

[model_providers.custom]
base_url = "https://custom.example/v1"
env_key = "CUSTOM_API_KEY"
requires_openai_auth = true
`))
	writeCodexTestAuthFile(t, filepath.Join(codexHome, codexAuthFileName), "auth-key")

	var resolver codexAPIConnectionResolver
	got := resolver.resolve(codexHome, func(string) string { return "provider-key" })
	requireCodexTestConnection(t, got, "auth-key", "https://custom.example/v1")
}

func TestCodexAPIConnectionResolverRejectsConfiguredProviderWithoutBaseURL(t *testing.T) {
	codexHome := t.TempDir()
	writeCodexTestFile(t, filepath.Join(codexHome, codexConfigFileName), []byte(`
model_provider = "custom"

[model_providers.custom]
env_key = "CUSTOM_API_KEY"
`))
	writeCodexTestAuthFile(t, filepath.Join(codexHome, codexAuthFileName), "auth-key")

	var resolver codexAPIConnectionResolver
	got := resolver.resolve(codexHome, func(string) string { return "provider-key" })
	requireCodexTestConnection(t, got, "", "")
}

func TestCodexAppServerEnvironmentOverridesInheritedAPIConnection(t *testing.T) {
	codexHome := t.TempDir()
	writeCodexTestFile(t, filepath.Join(codexHome, codexConfigFileName), []byte(`
model_provider = "custom"

[model_providers.custom]
base_url = "https://resolved.example/v1"
requires_openai_auth = true
`))
	writeCodexTestAuthFile(t, filepath.Join(codexHome, codexAuthFileName), "resolved-key")
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv(openAIAPIKeyEnvName, "inherited-key")
	t.Setenv(openAIBaseURLEnvName, "https://inherited.example/v1")
	t.Setenv("CODEKANBAN_TEST_ENV", "preserved")

	environment := codexAppServerEnvironment()
	if got, matched := codexTestEnvironmentValue(environment, openAIAPIKeyEnvName); got != "resolved-key" || matched != 1 {
		t.Fatalf("app-server API key = %q with %d entries, want resolved key exactly once", got, matched)
	}
	if got, matched := codexTestEnvironmentValue(environment, openAIBaseURLEnvName); got != "https://resolved.example/v1" || matched != 1 {
		t.Fatalf("app-server base URL = %q with %d entries, want resolved URL exactly once", got, matched)
	}
	if got, matched := codexTestEnvironmentValue(environment, "CODEKANBAN_TEST_ENV"); got != "preserved" || matched != 1 {
		t.Fatal("app-server environment dropped an unrelated inherited variable")
	}
}

func requireCodexTestConnection(t *testing.T, got codexAPIConnection, apiKey, baseURL string) {
	t.Helper()
	if got.APIKey != apiKey || got.BaseURL != baseURL {
		t.Fatalf("resolved connection = %#v, want API key %q and base URL %q", got, apiKey, baseURL)
	}
}

func codexTestEnvironmentValue(environment []string, name string) (string, int) {
	matched := 0
	value := ""
	for _, entry := range environment {
		separator := strings.IndexByte(entry, '=')
		if separator > 0 && environmentNamesEqual(entry[:separator], name) {
			matched++
			value = entry[separator+1:]
		}
	}
	return value, matched
}

func writeCodexTestAuthFile(t *testing.T, path, apiKey string) {
	t.Helper()
	raw, err := json.Marshal(codexAuthFile{OpenAIAPIKey: apiKey})
	if err != nil {
		t.Fatalf("marshal Codex auth file: %v", err)
	}
	writeCodexTestFile(t, path, raw)
}

func writeCodexTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(path), err)
	}
}

func codexTestFileModTime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", filepath.Base(path), err)
	}
	return info.ModTime()
}

func setCodexTestFileModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set %s mtime: %v", filepath.Base(path), err)
	}
}
