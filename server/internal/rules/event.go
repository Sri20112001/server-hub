// Package rules implements the Phase 2 notification rules engine.
//
// Design: the engine sits between alert/event generation and notification
// delivery. It decides "should this event notify?" — the existing notify
// package still decides "how to deliver?". With zero rules (or the
// NOTIFICATION_RULES_ENABLED flag off) the legacy pipeline runs untouched.
package rules

import (
	"fmt"
	"time"
)

// Supported event types. Each maps to real ServerHub behavior — never invent
// new ones without wiring a producer (see Engine.Evaluate call sites).
const (
	// EventAgentOffline fires when the offline loop marks a server OFFLINE.
	EventAgentOffline = "AGENT_OFFLINE"
	// EventAgentOnline fires when an offline alert resolves (server back).
	EventAgentOnline = "AGENT_ONLINE"
	// EventServerAlert fires on threshold alerts and Alertmanager firings.
	EventServerAlert = "SERVER_ALERT"
	// EventServerAlertResolved fires when such an alert resolves.
	EventServerAlertResolved = "SERVER_ALERT_RESOLVED"
)

// SupportedEventTypes lists every event type the engine accepts in rules.
var SupportedEventTypes = []string{
	EventAgentOffline,
	EventAgentOnline,
	EventServerAlert,
	EventServerAlertResolved,
}

// SupportedChannels lists deliverable channels. EMAIL, TELEGRAM and IN_APP
// reuse the existing notify package — no new channel implementations here.
// External channels (EMAIL, TELEGRAM) flow through the centralized delivery
// pipeline; IN_APP stays direct.
var SupportedChannels = []string{"EMAIL", "TELEGRAM", "IN_APP"}

// Event is the stable contract the rule engine consumes. Producers (alert
// loop, Alertmanager webhook) translate internal state into this shape;
// database rows are never exposed to rule evaluation.
type Event struct {
	// Type is one of SupportedEventTypes.
	Type string
	// ServerID is nil for host-level/non-server events.
	ServerID *uint
	// Severity is INFO, WARNING, or CRITICAL (uppercased by producers).
	Severity string
	// Condition identifies the trigger, e.g. "offline", "cpu_high", "HostHighCpu".
	Condition string
	// Value carries the measured value when applicable (e.g. CPU percent).
	Value float64
	// Threshold carries the breached threshold when applicable.
	Threshold float64
	// Message is the human-readable alert text.
	Message string
	// Fingerprint is the stable logical identity (Alertmanager fingerprint
	// or "server:<id>:<condition>" for internal alerts). Cooldown and
	// dedupe key off rule_id + fingerprint.
	Fingerprint string
	// Timestamp is the event time (defaults to now when zero).
	Timestamp time.Time
	// AlertID links the firing alert row when known (webhook path).
	AlertID *int64
}

// IsRecovery reports whether the event closes (rather than opens) an alert.
func (e Event) IsRecovery() bool {
	return e.Type == EventAgentOnline || e.Type == EventServerAlertResolved
}

// Fingerprint builds the stable logical identity for internal
// (non-Alertmanager) alerts: "server:<id>:<condition>".
func Fingerprint(serverID uint, condition string) string {
	return fmt.Sprintf("server:%d:%s", serverID, condition)
}
func validEventType(t string) bool {
	for _, s := range SupportedEventTypes {
		if t == s {
			return true
		}
	}
	return false
}

// validChannel reports whether c is a supported channel.
func validChannel(c string) bool {
	for _, s := range SupportedChannels {
		if c == s {
			return true
		}
	}
	return false
}
