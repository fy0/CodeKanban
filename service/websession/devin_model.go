package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"code-kanban/model/tables"
)

type devinACPConfigOption struct {
	ID           string `json:"id"`
	Category     string `json:"category"`
	CurrentValue string `json:"currentValue"`
	Options      []struct {
		Value string `json:"value"`
	} `json:"options"`
}

type devinACPModelSettings struct {
	ConfigOptions []devinACPConfigOption `json:"configOptions"`
	Models        *struct {
		CurrentModelID string `json:"currentModelId"`
	} `json:"models"`
}

func (s devinACPModelSettings) option(category string) *devinACPConfigOption {
	for i := range s.ConfigOptions {
		option := &s.ConfigOptions[i]
		if option.Category == category || option.ID == category {
			return option
		}
	}
	return nil
}

// Devin's current ACP model menu contains one representative variant per
// family (e.g. swe-2-high). Other variants use that value plus thought_level.
// Speed is another config option; context suffixes and Fusion sidekick
// configurations must still identify distinct model choices.
func devinACPModelParts(model string) (string, ReasoningEffort, string) {
	primary, sidekick, fusion := strings.Cut(model, "-sidekick-")
	if !strings.HasPrefix(model, "fusion-") {
		primary, sidekick, fusion = model, "", false
	}
	speed := "standard"
	for _, suffix := range []string{"-fast", "-priority", "-lightning"} {
		if strings.HasSuffix(primary, suffix) {
			primary = strings.TrimSuffix(primary, suffix)
			speed = "fast"
			break
		}
	}
	effort := devinReasoningEffortFromModel(primary)
	parts := strings.Split(primary, "-")
	for i, part := range parts {
		if part == string(effort) && effort != ReasoningEffortDefault {
			parts = append(parts[:i], parts[i+1:]...)
			break
		}
	}
	family := strings.Join(parts, "-")
	if fusion {
		family += "-sidekick-" + sidekick
	}
	return family, effort, speed
}

func devinACPModelValue(option *devinACPConfigOption, model string) string {
	for _, candidate := range option.Options {
		if candidate.Value == model {
			return model
		}
	}
	family, _, speed := devinACPModelParts(model)
	fallback := model
	for _, candidate := range option.Options {
		candidateFamily, _, candidateSpeed := devinACPModelParts(candidate.Value)
		if candidateFamily == family {
			if candidateSpeed == speed {
				return candidate.Value
			}
			fallback = candidate.Value
		}
	}
	return fallback // Let the agent report an unsupported selection explicitly.
}

func setDevinConfigOption(ctx context.Context, client *devinACPClient, nativeSessionID string, option *devinACPConfigOption, value string) (devinACPModelSettings, error) {
	raw, err := client.request(ctx, "session/set_config_option", map[string]any{
		"sessionId": nativeSessionID,
		"configId":  option.ID,
		"value":     value,
	})
	if err != nil {
		return devinACPModelSettings{}, fmt.Errorf("set %s to %q: %w", option.ID, value, err)
	}
	var settings devinACPModelSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return settings, fmt.Errorf("decode Devin model settings: %w", err)
	}
	if applied := settings.option(option.ID); applied == nil || applied.CurrentValue != value {
		return settings, fmt.Errorf("Devin did not apply %s %q", option.ID, value)
	}
	return settings, nil
}

func applyDevinSessionModel(ctx context.Context, client *devinACPClient, session tables.WebSessionTable, nativeSessionID string, raw json.RawMessage) error {
	model := strings.TrimSpace(session.Model)
	if model == "" {
		return nil
	}
	var settings devinACPModelSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return fmt.Errorf("decode Devin session model: %w", err)
	}
	option := settings.option("model")
	if option == nil {
		// Older ACP agents advertise models instead of configOptions.
		if settings.Models != nil && settings.Models.CurrentModelID == model {
			return nil
		}
		_, err := client.request(ctx, "session/set_model", map[string]any{
			"sessionId": nativeSessionID,
			"modelId":   model,
		})
		return err
	}
	value := devinACPModelValue(option, model)
	if option.CurrentValue != value {
		var err error
		settings, err = setDevinConfigOption(ctx, client, nativeSessionID, option, value)
		if err != nil {
			return err
		}
	}

	// The catalog ID carries the selected effort. Prefer it over a possibly
	// stale reasoning_effort field from the previous model's separate update.
	_, effort, speed := devinACPModelParts(model)
	_, valueEffort, valueSpeed := devinACPModelParts(value)
	if effort == ReasoningEffortDefault {
		effort = normalizeReasoningEffort(ReasoningEffort(session.ReasoningEffort))
	}
	thinking := settings.option("thought_level")
	if thinking == nil {
		if effort != ReasoningEffortDefault && effort != valueEffort {
			return fmt.Errorf("Devin cannot apply the thinking level for model %q", model)
		}
	} else if effort != ReasoningEffortDefault && thinking.CurrentValue != string(effort) {
		var err error
		settings, err = setDevinConfigOption(ctx, client, nativeSessionID, thinking, string(effort))
		if err != nil {
			return err
		}
	}
	if option := settings.option("speed"); option != nil {
		if option.CurrentValue != speed {
			_, err := setDevinConfigOption(ctx, client, nativeSessionID, option, speed)
			return err
		}
	} else if speed != valueSpeed {
		return fmt.Errorf("Devin cannot apply the speed for model %q", model)
	}
	return nil
}
