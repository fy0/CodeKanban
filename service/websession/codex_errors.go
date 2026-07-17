package websession

import "strings"

const (
	codexCyberPolicyErrorCode    = "cyber_policy"
	codexCyberPolicyFallbackText = "This request has been flagged for possible cybersecurity risk."
)

func normalizeCodexErrorInfo(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "cyberpolicy":
		return codexCyberPolicyErrorCode
	default:
		return normalized
	}
}

func codexErrorInfo(record map[string]any) string {
	if len(record) == 0 {
		return ""
	}
	for _, key := range []string{"codex_error_info", "codexErrorInfo"} {
		if value := normalizeCodexErrorInfo(stringValue(record[key])); value != "" {
			return value
		}
	}
	for _, key := range []string{"error", "turn"} {
		if value := codexErrorInfo(decodeRawObject(record[key])); value != "" {
			return value
		}
	}
	return ""
}

func isCodexCyberPolicyError(record map[string]any) bool {
	return codexErrorInfo(record) == codexCyberPolicyErrorCode
}

func codexErrorMessage(record map[string]any) string {
	if len(record) == 0 {
		return ""
	}
	if message := strings.TrimSpace(firstNonEmpty(
		stringValue(record["message"]),
		stringValue(record["additionalDetails"]),
		stringValue(record["additional_details"]),
	)); message != "" {
		return message
	}
	if errorRecord := decodeRawObject(record["error"]); len(errorRecord) > 0 {
		return codexErrorMessage(errorRecord)
	}
	return ""
}
