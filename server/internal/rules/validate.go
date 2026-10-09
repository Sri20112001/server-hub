package rules

import (
	"fmt"
	"strings"
)

// MaxCooldownSeconds bounds cooldowns (30 days).
const MaxCooldownSeconds = 30 * 24 * 3600

// MaxRepeats bounds repeat reminders per incident (0 = policy default).
const MaxRepeats = 100

// DigestIntervalsMin are the allowed digest windows.
var DigestIntervalsMin = []int{15, 30, 60, 180, 720, 1440}

// GroupByKeys are the label keys digest batches may split on.
var GroupByKeys = []string{"severity", "alertname", "condition", "server", "service", "environment"}

// ValidateRule checks a complete rule definition. Called on create and on
// update (after merging patches over the stored row), so stored rows are
// always valid. Returns a human-readable error for HTTP 400 responses.
func ValidateRule(name, description, eventType, severity, conditionJSON string, channels []string, cooldownSeconds int) error {
	return ValidateRuleFull(RuleParams{
		Name: name, Description: description, EventType: eventType,
		Severity: severity, ConditionJSON: conditionJSON,
		Channels: channels, CooldownSeconds: cooldownSeconds,
	})
}

// RuleParams carries every validated rule field.
type RuleParams struct {
	Name, Description, EventType, Severity, ConditionJSON string
	Channels                                              []string
	CooldownSeconds, RepeatIntervalSec, MaxRepeats         int
	DigestMode                                            string
	DigestIntervalMin                                     int
	GroupBy                                               []string
}

// ValidateRuleFull validates the complete rule definition including
// repeat/digest policy.
func ValidateRuleFull(p RuleParams) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(p.Name) > 100 {
		return fmt.Errorf("name too long (max 100)")
	}
	if len(p.Description) > 500 {
		return fmt.Errorf("description too long (max 500)")
	}
	if !validEventType(p.EventType) {
		return fmt.Errorf("unsupported event type %q (use %s)", p.EventType, strings.Join(SupportedEventTypes, "|"))
	}
	if p.Severity != "" {
		switch strings.ToUpper(p.Severity) {
		case "INFO", "WARNING", "CRITICAL":
		default:
			return fmt.Errorf("severity must be empty or INFO|WARNING|CRITICAL")
		}
	}
	if _, err := ParseCondition(p.ConditionJSON); err != nil {
		return err
	}
	if len(p.Channels) == 0 {
		return fmt.Errorf("at least one channel is required (use EMAIL|TELEGRAM|IN_APP)")
	}
	for _, c := range p.Channels {
		if !validChannel(c) {
			return fmt.Errorf("unsupported channel %q (use EMAIL|TELEGRAM|IN_APP)", c)
		}
	}
	if p.CooldownSeconds < 0 || p.CooldownSeconds > MaxCooldownSeconds {
		return fmt.Errorf("cooldown must be 0..%d seconds", MaxCooldownSeconds)
	}
	if p.RepeatIntervalSec < 0 || p.RepeatIntervalSec > MaxCooldownSeconds {
		return fmt.Errorf("repeat interval must be 0..%d seconds", MaxCooldownSeconds)
	}
	if p.MaxRepeats < 0 || p.MaxRepeats > MaxRepeats {
		return fmt.Errorf("max repeats must be 0..%d", MaxRepeats)
	}
	switch p.DigestMode {
	case "", DigestImmediate, DigestDigest:
	default:
		return fmt.Errorf("digest mode must be immediate or digest")
	}
	if p.DigestMode == DigestDigest {
		ok := false
		for _, v := range DigestIntervalsMin {
			if v == p.DigestIntervalMin {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("digest interval must be one of 15|30|60|180|720|1440 minutes")
		}
	}
	if len(p.GroupBy) > 3 {
		return fmt.Errorf("at most 3 group-by keys allowed")
	}
	for _, k := range p.GroupBy {
		ok := false
		for _, allowed := range GroupByKeys {
			if k == allowed {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("unsupported group-by key %q", k)
		}
	}
	return nil
}

// Digest mode constants mirror the delivery package without importing it
// (rules must not depend on delivery; main wires the two).
const (
	DigestImmediate = "immediate"
	DigestDigest    = "digest"
)

// NormalizeGroupBy keeps valid group-by keys (max 3), joined for storage.
// Invalid keys are dropped (ValidateRuleFull rejects them outright on
// direct calls; handlers normalize-then-validate so stored rows stay clean).
func NormalizeGroupBy(keys []string) string {
	var out []string
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		allowed := false
		for _, a := range GroupByKeys {
			if k == a {
				allowed = true
				break
			}
		}
		if allowed && !contains(out, k) && len(out) < 3 {
			out = append(out, k)
		}
	}
	return strings.Join(out, ",")
}

// SplitGroupBy splits a stored group-by list.
func SplitGroupBy(raw string) []string {
	var out []string
	for _, k := range strings.Split(raw, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out = append(out, k)
		}
	}
	return out
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
