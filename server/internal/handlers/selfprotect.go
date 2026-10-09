package handlers

import (
	"os"
	"strings"
)

// Self-protection: the backend holds the Docker socket (≈ host root), so
// destructive operations must refuse ServerHub's own container and the
// infrastructure containers it depends on. Recovery stays possible via the
// host's docker CLI — just never through this API (no foot-gun outage,
// no container-escape stepping stone).
var protectedInfraNames = map[string]bool{
	"serverhub":                 true,
	"serverhub-postgres":        true,
	"serverhub-prometheus":      true,
	"serverhub-alertmanager":    true,
	"serverhub-node-exporter":   true,
	"serverhub-cadvisor":        true,
}

// protectedTarget returns a non-empty reason when the (name, id) container
// must not receive destructive operations. hostname is the API server's own
// hostname (inside Docker: the short container ID).
func protectedTarget(name, id, hostname string) string {
	n := strings.Trim(strings.ToLower(strings.TrimSpace(name)), "/")
	if protectedInfraNames[n] {
		return "refusing destructive operation on protected infrastructure container " + n
	}
	if isSelfContainerID(strings.ToLower(strings.TrimSpace(id)), strings.ToLower(strings.TrimSpace(hostname))) {
		return "refusing destructive operation on the ServerHub container itself"
	}
	return ""
}

// denyContainerTarget checks the live hostname. Empty reason = allowed.
func denyContainerTarget(name, id string) string {
	hostname, _ := os.Hostname()
	return protectedTarget(name, id, hostname)
}

func isHexID(s string) bool {
	if len(s) < 8 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// isSelfContainerID matches a target against our own container ID in any
// common form: full 64-hex ID, 12-char short ID, or exact hostname.
func isSelfContainerID(target, hostname string) bool {
	if target == "" || hostname == "" {
		return false
	}
	if !isHexID(hostname) && !isHexID(target) {
		// Neither looks like a container ID (e.g. dev machine without
		// Docker): only exact matches count, never prefixes.
		return target == hostname
	}
	if target == hostname {
		return true
	}
	// Prefix match only when the SHORTER side is a substantial ID prefix.
	short, long := target, hostname
	if len(short) > len(long) {
		short, long = long, short
	}
	return len(short) >= 8 && isHexID(short) && strings.HasPrefix(long, short)
}
