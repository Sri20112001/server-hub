package rules

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Condition is a single structured predicate over NotificationEvent fields.
// No expression language, no eval: field/operator come from closed sets and
// values are type-checked against the field kind.
type Condition struct {
	// Field is one of: severity, condition, value, serverId.
	Field string `json:"field"`
	// Operator: eq, neq for all; contains for strings;
	// gt, gte, lt, lte for numbers; in for both (array value).
	Operator string `json:"operator"`
	// Value is a string, number, or array of those (for in).
	Value any `json:"value"`
}

// ParseCondition validates raw JSON into a Condition. Empty input means "no
// condition". Anything else must match the schema exactly.
func ParseCondition(raw string) (*Condition, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var c Condition
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("condition must be JSON {field, operator, value}: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Condition) validate() error {
	stringField := c.Field == "severity" || c.Field == "condition"
	numberField := c.Field == "value" || c.Field == "serverId"
	if !stringField && !numberField {
		return fmt.Errorf("unsupported condition field %q (use severity|condition|value|serverId)", c.Field)
	}
	switch c.Operator {
	case "eq", "neq":
	case "contains":
		if !stringField {
			return fmt.Errorf("operator %q needs a string field", c.Operator)
		}
	case "gt", "gte", "lt", "lte":
		if !numberField {
			return fmt.Errorf("operator %q needs a numeric field", c.Operator)
		}
	case "in":
	default:
		return fmt.Errorf("unsupported operator %q (use eq|neq|gt|gte|lt|lte|contains|in)", c.Operator)
	}
	if c.Operator == "in" {
		arr, ok := c.Value.([]any)
		if !ok || len(arr) == 0 {
			return fmt.Errorf("operator \"in\" needs a non-empty array value")
		}
		for _, v := range arr {
			if err := checkValueKind(c.Field, v); err != nil {
				return err
			}
		}
		return nil
	}
	return checkValueKind(c.Field, c.Value)
}

func checkValueKind(field string, v any) error {
	switch field {
	case "severity", "condition":
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return fmt.Errorf("field %q needs a non-empty string value", field)
		}
		if field == "severity" {
			switch strings.ToUpper(s) {
			case "INFO", "WARNING", "CRITICAL":
			default:
				return fmt.Errorf("severity must be INFO|WARNING|CRITICAL")
			}
		}
	case "value", "serverId":
		if _, ok := toFloat(v); !ok {
			return fmt.Errorf("field %q needs a numeric value", field)
		}
	}
	return nil
}

// toFloat coerces JSON numbers (json.Number or float64) to float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// Matches evaluates the condition against an event. A nil condition matches.
func (c *Condition) Matches(ev Event) bool {
	if c == nil {
		return true
	}
	switch c.Field {
	case "severity":
		return matchString(strings.ToUpper(ev.Severity), c.Operator, c.Value)
	case "condition":
		return matchString(ev.Condition, c.Operator, c.Value)
	case "value":
		return matchNumber(ev.Value, c.Operator, c.Value)
	case "serverId":
		var id float64
		if ev.ServerID != nil {
			id = float64(*ev.ServerID)
		} else {
			id = -1 // nil never equals a real id; neq still works
		}
		return matchNumber(id, c.Operator, c.Value)
	}
	return false
}

func matchString(s, op string, v any) bool {
	if op == "in" {
		arr, _ := v.([]any)
		for _, item := range arr {
			if str, ok := item.(string); ok && strings.EqualFold(s, str) {
				return true
			}
		}
		return false
	}
	want, _ := v.(string)
	switch op {
	case "eq":
		return strings.EqualFold(s, want)
	case "neq":
		return !strings.EqualFold(s, want)
	case "contains":
		return strings.Contains(strings.ToLower(s), strings.ToLower(want))
	}
	return false
}

func matchNumber(n float64, op string, v any) bool {
	if op == "in" {
		arr, _ := v.([]any)
		for _, item := range arr {
			if f, ok := toFloat(item); ok && f == n {
				return true
			}
		}
		return false
	}
	want, ok := toFloat(v)
	if !ok {
		return false
	}
	switch op {
	case "eq":
		return n == want
	case "neq":
		return n != want
	case "gt":
		return n > want
	case "gte":
		return n >= want
	case "lt":
		return n < want
	case "lte":
		return n <= want
	}
	return false
}
