package rules

import (
	"fmt"
	"strings"
)

// MaxCooldownSeconds bounds cooldowns (30 days).
const MaxCooldownSeconds = 30 * 24 * 3600

// ValidateRule checks a complete rule definition. Called on create and on
// update (after merging patches over the stored row), so stored rows are
// always valid. Returns a human-readable error for HTTP 400 responses.
func ValidateRule(name, description, eventType, severity, conditionJSON string, channels []string, cooldownSeconds int) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 100 {
		return fmt.Errorf("name too long (max 100)")
	}
	if len(description) > 500 {
		return fmt.Errorf("description too long (max 500)")
	}
	if !validEventType(eventType) {
		return fmt.Errorf("unsupported event type %q (use %s)", eventType, strings.Join(SupportedEventTypes, "|"))
	}
	if severity != "" {
		switch strings.ToUpper(severity) {
		case "INFO", "WARNING", "CRITICAL":
		default:
			return fmt.Errorf("severity must be empty or INFO|WARNING|CRITICAL")
		}
	}
	if _, err := ParseCondition(conditionJSON); err != nil {
		return err
	}
	if len(channels) == 0 {
		return fmt.Errorf("at least one channel is required (use EMAIL|IN_APP)")
	}
	for _, c := range channels {
		if !validChannel(c) {
			return fmt.Errorf("unsupported channel %q (use EMAIL|IN_APP)", c)
		}
	}
	if cooldownSeconds < 0 || cooldownSeconds > MaxCooldownSeconds {
		return fmt.Errorf("cooldown must be 0..%d seconds", MaxCooldownSeconds)
	}
	return nil
}

// NormalizeChannels uppercases, dedupes, and joins a channel list for storage.
func NormalizeChannels(channels []string) string {
	var out []string
	for _, c := range channels {
		c = strings.ToUpper(strings.TrimSpace(c))
		if validChannel(c) && !contains(out, c) {
			out = append(out, c)
		}
	}
	return strings.Join(out, ",")
}

// NormalizeSeverity uppercases an optional severity matcher ("" stays "").
func NormalizeSeverity(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}
