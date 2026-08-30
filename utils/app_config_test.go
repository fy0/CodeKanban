package utils

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/knadh/koanf/v2"
)

func TestNormalizePageTitle(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default for empty", input: "", want: DefaultPageTitle},
		{name: "default for blank", input: "   ", want: DefaultPageTitle},
		{name: "trim custom title", input: "  工作实例 🚀  ", want: "工作实例 🚀"},
		{name: "allow max length", input: strings.Repeat("界", MaxPageTitleRunes), want: strings.Repeat("界", MaxPageTitleRunes)},
		{name: "reject over max length", input: strings.Repeat("界", MaxPageTitleRunes+1), wantErr: true},
		{name: "reject control characters", input: "Bad\nTitle", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePageTitle(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizePageTitle(%q) expected an error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePageTitle(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizePageTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReadConfigDefaultsPageTitleWhenMissing(t *testing.T) {
	content := "ui: {}\n"
	config, _ := readDailyTipTestConfig(t, &content)
	if got := config.UI.PageTitle; got != DefaultPageTitle {
		t.Fatalf("ui.pageTitle = %q, want %q", got, DefaultPageTitle)
	}
}

func TestNormalizeWebSessionQuickInputConfig(t *testing.T) {
	input := WebSessionQuickInputConfig{
		Pinned: []string{"  Alpha  ", "", "Beta", "Alpha"},
		Recent: []string{" One ", "Two", "One", "", "Three", "Four", "Five", "Six", "Seven"},
		RecentByProject: map[string][]string{
			" project-1 ": {" One ", "Two", "One"},
			" ":           {"ignored"},
		},
	}

	got := NormalizeWebSessionQuickInputConfig(input)
	want := WebSessionQuickInputConfig{
		Pinned: []string{"Alpha", "Beta"},
		Recent: []string{"One", "Two", "Three", "Four", "Five", "Six", "Seven"},
		RecentByProject: map[string][]string{
			"project-1": {"One", "Two"},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeWebSessionQuickInputConfig() = %#v, want %#v", got, want)
	}
}

func TestNormalizeWebSessionQuickInputConfigEmpty(t *testing.T) {
	got := NormalizeWebSessionQuickInputConfig(WebSessionQuickInputConfig{})
	want := WebSessionQuickInputConfig{
		Pinned:          []string{},
		Recent:          []string{},
		RecentByProject: map[string][]string{},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeWebSessionQuickInputConfig() = %#v, want %#v", got, want)
	}
}

func TestNormalizeDeveloperConfigDefaultsActiveCallTimeout(t *testing.T) {
	got := NormalizeDeveloperConfig(DeveloperConfig{})
	if got.WebSessionCodexDefaultModel != WebSessionCodexDefaultSetting {
		t.Fatalf("expected Codex model sentinel %q, got %q", WebSessionCodexDefaultSetting, got.WebSessionCodexDefaultModel)
	}
	if got.WebSessionCodexDefaultReasoningEffort != WebSessionCodexDefaultSetting {
		t.Fatalf(
			"expected Codex reasoning sentinel %q, got %q",
			WebSessionCodexDefaultSetting,
			got.WebSessionCodexDefaultReasoningEffort,
		)
	}
	if got.WebSessionCodexDefaultPermissionLevel != WebSessionCodexDefaultSetting {
		t.Fatalf("expected Codex permission sentinel %q, got %q", WebSessionCodexDefaultSetting, got.WebSessionCodexDefaultPermissionLevel)
	}
	if got.WebSessionCodexDefaultSyncMode != WebSessionCodexDefaultSetting {
		t.Fatalf("expected Codex sync sentinel %q, got %q", WebSessionCodexDefaultSetting, got.WebSessionCodexDefaultSyncMode)
	}
	if got.WebSessionAutoRetryDefaults != defaultWebSessionAutoRetryDefaultsConfig {
		t.Fatalf("expected default auto-retry config, got %#v", got.WebSessionAutoRetryDefaults)
	}
	if got.WebSessionActiveCallTimeout.EnabledMode != SettingModeDefault {
		t.Fatalf("expected enabled mode default, got %q", got.WebSessionActiveCallTimeout.EnabledMode)
	}
	if got.WebSessionActiveCallTimeout.TimeoutMode != WebSessionActiveCallTimeoutModeDefault {
		t.Fatalf("expected timeout mode default, got %q", got.WebSessionActiveCallTimeout.TimeoutMode)
	}
	if got.WebSessionActiveCallTimeout.CustomTimeoutSeconds != DefaultWebSessionActiveCallTimeoutSeconds {
		t.Fatalf(
			"expected custom timeout %d, got %d",
			DefaultWebSessionActiveCallTimeoutSeconds,
			got.WebSessionActiveCallTimeout.CustomTimeoutSeconds,
		)
	}
	if got.WebSessionActiveCallTimeout.PromptTemplate != DefaultWebSessionActiveCallTimeoutPrompt {
		t.Fatalf("expected default prompt template, got %q", got.WebSessionActiveCallTimeout.PromptTemplate)
	}
	if !got.WebSessionActiveCallTimeout.CallKinds.UseDefault {
		t.Fatal("expected useDefault call kinds to be enabled")
	}
	if !got.WebSessionActiveCallTimeout.CallKinds.MCP ||
		got.WebSessionActiveCallTimeout.CallKinds.Command ||
		!got.WebSessionActiveCallTimeout.CallKinds.Tool {
		t.Fatalf("expected default call kinds to exclude command, got %#v", got.WebSessionActiveCallTimeout.CallKinds)
	}
}

func TestNormalizeWebSessionAutoRetryDefaultsConfig(t *testing.T) {
	got := NormalizeWebSessionAutoRetryDefaultsConfig(WebSessionAutoRetryDefaultsConfig{
		Scope:                    " ALL_FAILURES ",
		Preset:                   " SUSTAIN_60S ",
		MaxAttempts:              200,
		DispatchPendingOnFailure: true,
	})
	want := WebSessionAutoRetryDefaultsConfig{
		Scope:                    "all_failures",
		Preset:                   "sustain_60s",
		MaxAttempts:              MaxWebSessionAutoRetryMaxAttempts,
		DispatchPendingOnFailure: true,
	}
	if got != want {
		t.Fatalf("NormalizeWebSessionAutoRetryDefaultsConfig() = %#v, want %#v", got, want)
	}
}

func TestNormalizeDeveloperConfigPreservesCustomCodexDefaults(t *testing.T) {
	got := NormalizeDeveloperConfig(DeveloperConfig{
		WebSessionCodexDefaultModel:           "  custom-codex-model  ",
		WebSessionCodexDefaultReasoningEffort: " HIGH ",
		WebSessionCodexDefaultPermissionLevel: " YOLO ",
		WebSessionCodexDefaultSyncMode:        " DEEP ",
	})
	if got.WebSessionCodexDefaultModel != "custom-codex-model" {
		t.Fatalf("expected trimmed custom model, got %q", got.WebSessionCodexDefaultModel)
	}
	if got.WebSessionCodexDefaultReasoningEffort != "high" {
		t.Fatalf("expected normalized high effort, got %q", got.WebSessionCodexDefaultReasoningEffort)
	}
	if got.WebSessionCodexDefaultPermissionLevel != "yolo" {
		t.Fatalf("expected normalized yolo permission, got %q", got.WebSessionCodexDefaultPermissionLevel)
	}
	if got.WebSessionCodexDefaultSyncMode != "deep" {
		t.Fatalf("expected normalized deep sync mode, got %q", got.WebSessionCodexDefaultSyncMode)
	}

	invalid := NormalizeDeveloperConfig(DeveloperConfig{
		WebSessionCodexDefaultReasoningEffort: "unsupported",
		WebSessionCodexDefaultPermissionLevel: "unsupported",
		WebSessionCodexDefaultSyncMode:        "unsupported",
	})
	if invalid.WebSessionCodexDefaultReasoningEffort != WebSessionCodexDefaultSetting {
		t.Fatalf(
			"expected invalid effort to fall back to %q, got %q",
			WebSessionCodexDefaultSetting,
			invalid.WebSessionCodexDefaultReasoningEffort,
		)
	}
	if invalid.WebSessionCodexDefaultPermissionLevel != WebSessionCodexDefaultSetting ||
		invalid.WebSessionCodexDefaultSyncMode != WebSessionCodexDefaultSetting {
		t.Fatalf("expected invalid Codex settings to use sentinels, got %#v", invalid)
	}

	sentinels := NormalizeDeveloperConfig(DeveloperConfig{
		WebSessionCodexDefaultModel:           " DEFAULT ",
		WebSessionCodexDefaultReasoningEffort: WebSessionCodexModelDefaultEffort,
		WebSessionCodexDefaultPermissionLevel: WebSessionCodexStandardPermission,
		WebSessionCodexDefaultSyncMode:        WebSessionCodexDefaultSetting,
	})
	if sentinels.WebSessionCodexDefaultModel != WebSessionCodexDefaultSetting ||
		sentinels.WebSessionCodexDefaultReasoningEffort != WebSessionCodexModelDefaultEffort ||
		sentinels.WebSessionCodexDefaultPermissionLevel != WebSessionCodexStandardPermission ||
		sentinels.WebSessionCodexDefaultSyncMode != WebSessionCodexDefaultSetting {
		t.Fatalf("expected explicit sentinel settings to survive normalization, got %#v", sentinels)
	}
}

func TestNormalizeWebSessionActiveCallTimeoutConfigClampsMinAndTrims(t *testing.T) {
	got := NormalizeWebSessionActiveCallTimeoutConfig(WebSessionActiveCallTimeoutConfig{
		EnabledMode:          SettingMode("ON"),
		TimeoutMode:          WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 2,
		PromptTemplate:       "  custom ${call} / ${duration}  ",
		CallKinds: WebSessionActiveCallTimeoutKindsConfig{
			UseDefault: false,
			MCP:        true,
		},
	})
	if got.EnabledMode != SettingModeOn {
		t.Fatalf("expected enabled mode on, got %q", got.EnabledMode)
	}
	if got.TimeoutMode != WebSessionActiveCallTimeoutModeCustom {
		t.Fatalf("expected timeout mode custom, got %q", got.TimeoutMode)
	}
	if got.CustomTimeoutSeconds != minWebSessionActiveCallTimeoutSeconds {
		t.Fatalf(
			"expected timeout to clamp to %d, got %d",
			minWebSessionActiveCallTimeoutSeconds,
			got.CustomTimeoutSeconds,
		)
	}
	if got.PromptTemplate != "custom ${call} / ${duration}" {
		t.Fatalf("expected prompt template to be trimmed, got %q", got.PromptTemplate)
	}
	if got.CallKinds.UseDefault {
		t.Fatal("expected useDefault to remain disabled")
	}
	if !got.CallKinds.MCP || got.CallKinds.Command || got.CallKinds.Tool {
		t.Fatalf("expected only MCP to remain enabled, got %#v", got.CallKinds)
	}
}

func TestNormalizeWebSessionActiveCallTimeoutConfigPreservesLargeCustomValue(t *testing.T) {
	got := NormalizeWebSessionActiveCallTimeoutConfig(WebSessionActiveCallTimeoutConfig{
		TimeoutMode:          WebSessionActiveCallTimeoutModeCustom,
		CustomTimeoutSeconds: 7200,
	})
	if got.CustomTimeoutSeconds != 7200 {
		t.Fatalf("expected large custom timeout to be preserved, got %d", got.CustomTimeoutSeconds)
	}
}

func TestEffectiveWebSessionActiveCallTimeoutSecondsUsesDefaultTier(t *testing.T) {
	got := effectiveWebSessionActiveCallTimeoutSeconds(WebSessionActiveCallTimeoutConfig{
		TimeoutMode:          WebSessionActiveCallTimeoutModeDefault,
		CustomTimeoutSeconds: 3600,
	})
	if got != DefaultWebSessionActiveCallTimeoutSeconds {
		t.Fatalf("expected default tier timeout %d, got %d", DefaultWebSessionActiveCallTimeoutSeconds, got)
	}
}

func TestMergeDeveloperConfigPreservesNestedTimeoutConfigForLegacyPayloads(t *testing.T) {
	current := NormalizeDeveloperConfig(DeveloperConfig{
		WebSessionCodexDefaultModel:           "custom-model",
		WebSessionCodexDefaultReasoningEffort: "medium",
		WebSessionCodexDefaultPermissionLevel: "yolo",
		WebSessionCodexDefaultSyncMode:        "deep",
		WebSessionAutoRetryDefaults: WebSessionAutoRetryDefaultsConfig{
			Scope:                    "all_failures",
			Preset:                   "sustain_60s",
			MaxAttempts:              8,
			DispatchPendingOnFailure: true,
		},
		WebSessionActiveCallTimeout: WebSessionActiveCallTimeoutConfig{
			EnabledMode:          SettingModeOff,
			TimeoutMode:          WebSessionActiveCallTimeoutModeCustom,
			CustomTimeoutSeconds: 120,
			PromptTemplate:       "resume ${call}",
			CallKinds: WebSessionActiveCallTimeoutKindsConfig{
				UseDefault: false,
				Command:    true,
			},
		},
	})

	merged := MergeDeveloperConfig(current, DeveloperConfig{
		EnableTerminalScrollback: true,
	})

	if !merged.EnableTerminalScrollback {
		t.Fatalf("expected top-level developer config fields to update, got %#v", merged)
	}
	if merged.WebSessionActiveCallTimeout != current.WebSessionActiveCallTimeout {
		t.Fatalf("expected nested timeout config to be preserved, got %#v", merged.WebSessionActiveCallTimeout)
	}
	if merged.WebSessionAutoRetryDefaults != current.WebSessionAutoRetryDefaults {
		t.Fatalf("expected nested auto-retry config to be preserved, got %#v", merged.WebSessionAutoRetryDefaults)
	}
	if merged.WebSessionCodexDefaultModel != current.WebSessionCodexDefaultModel ||
		merged.WebSessionCodexDefaultReasoningEffort != current.WebSessionCodexDefaultReasoningEffort ||
		merged.WebSessionCodexDefaultPermissionLevel != current.WebSessionCodexDefaultPermissionLevel ||
		merged.WebSessionCodexDefaultSyncMode != current.WebSessionCodexDefaultSyncMode {
		t.Fatalf("expected Codex defaults to be preserved for a legacy payload, got %#v", merged)
	}
}

func TestReadConfigMigratesLegacyActiveCallTimeoutToDefaultTier(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	legacyConfig := `
developer:
  webSessionActiveCallTimeout:
    enabledMode: on
    timeoutSeconds: 300
    callKinds:
      useDefault: true
      mcp: true
      command: true
      tool: true
`
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(legacyConfig)+"\n"), 0o644); err != nil {
		t.Fatalf("write legacy config failed: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	oldStore := configStore
	oldActivePath := activeConfigPath
	oldActiveConfigExisted := activeConfigExisted
	oldUseHomeData := useHomeData
	configStore = koanf.New(".")
	activeConfigPath = ""
	activeConfigExisted = false
	useHomeData = false
	t.Cleanup(func() {
		configStore = oldStore
		activeConfigPath = oldActivePath
		activeConfigExisted = oldActiveConfigExisted
		useHomeData = oldUseHomeData
	})

	config := ReadConfig()
	got := config.Developer.WebSessionActiveCallTimeout
	if got.TimeoutMode != WebSessionActiveCallTimeoutModeDefault {
		t.Fatalf("expected migrated timeout mode default, got %q", got.TimeoutMode)
	}
	if effectiveWebSessionActiveCallTimeoutSeconds(got) != DefaultWebSessionActiveCallTimeoutSeconds {
		t.Fatalf(
			"expected effective timeout %d, got %d",
			DefaultWebSessionActiveCallTimeoutSeconds,
			effectiveWebSessionActiveCallTimeoutSeconds(got),
		)
	}
	if !got.CallKinds.UseDefault || !got.CallKinds.MCP || got.CallKinds.Command || !got.CallKinds.Tool {
		t.Fatalf("expected migrated default call kinds without command, got %#v", got.CallKinds)
	}

	rewritten, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	content := string(rewritten)
	if strings.Contains(content, "timeoutSeconds:") {
		t.Fatalf("expected legacy timeoutSeconds key to be removed, got:\n%s", content)
	}
	if !strings.Contains(content, "timeoutMode: default") {
		t.Fatalf("expected timeoutMode to be rewritten, got:\n%s", content)
	}
	if !strings.Contains(content, "customTimeoutSeconds: 120") {
		t.Fatalf("expected customTimeoutSeconds to be rewritten, got:\n%s", content)
	}
	if !strings.Contains(content, "command: false") {
		t.Fatalf("expected default call kinds to rewrite command=false, got:\n%s", content)
	}
}

func TestReadConfigWritesBootstrapOnlyConfigOnFirstStart(t *testing.T) {
	config, configPath := readDailyTipTestConfig(t, nil)
	if config.UI.DailyTipEnabled {
		t.Fatal("expected ui.dailyTipEnabled to default to false on first start")
	}

	rewritten, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read generated config failed: %v", err)
	}
	if strings.Contains(string(rewritten), "dailyTipEnabled") || strings.Contains(string(rewritten), "\nui:") {
		t.Fatalf("expected generated config to exclude runtime UI settings, got:\n%s", rewritten)
	}
}

func TestReadConfigDefaultsDailyTipDisabledWhenMissing(t *testing.T) {
	content := "ui: {}\n"
	config, _ := readDailyTipTestConfig(t, &content)
	if config.UI.DailyTipEnabled {
		t.Fatal("expected ui.dailyTipEnabled to default to false when config key is missing")
	}
}

func TestReadConfigPreservesExplicitDailyTipEnabled(t *testing.T) {
	content := "ui:\n  dailyTipEnabled: true\n"
	config, _ := readDailyTipTestConfig(t, &content)
	if !config.UI.DailyTipEnabled {
		t.Fatal("expected explicit ui.dailyTipEnabled=true to be preserved")
	}
}

func readDailyTipTestConfig(t *testing.T, content *string) (*AppConfig, string) {
	t.Helper()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if content != nil {
		if err := os.WriteFile(configPath, []byte(*content), 0o644); err != nil {
			t.Fatalf("write config failed: %v", err)
		}
	} else {
		configPath = filepath.Join(tempDir, "data", "config.yaml")
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	oldStore := configStore
	oldActivePath := activeConfigPath
	oldActiveConfigExisted := activeConfigExisted
	oldUseHomeData := useHomeData
	configStore = koanf.New(".")
	activeConfigPath = ""
	activeConfigExisted = false
	useHomeData = false
	t.Cleanup(func() {
		configStore = oldStore
		activeConfigPath = oldActivePath
		activeConfigExisted = oldActiveConfigExisted
		useHomeData = oldUseHomeData
	})

	return ReadConfig(), configPath
}
