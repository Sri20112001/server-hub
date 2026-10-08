package monitoring

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Supported monitoring windows for viewer-facing range APIs. The largest
// window (7d) bounds every range query the UI can generate.
var rangeSeconds = map[string]int64{
	"1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800,
}

// MinStepSeconds floors range steps: below the scrape cadence steps only
// multiply returned points without adding information.
const MinStepSeconds int64 = 15

// MaxRangeSeconds caps the query window (7d, the largest UI range).
const MaxRangeSeconds int64 = 604800

// RangeSeconds resolves a range label like "1h" to seconds.
func RangeSeconds(rangeStr string) (int64, bool) {
	dur, ok := rangeSeconds[rangeStr]
	return dur, ok
}

// promDuration matches Prometheus duration literals: bare numbers are
// seconds; otherwise a sequence of <number><unit> with unit in
// ms|s|m|h|d|w|y (same units Prometheus itself accepts).
var promDuration = regexp.MustCompile(`^([0-9]+(ms|s|m|h|d|w|y))+$|^0$`)

// ParseStepSeconds parses a Prometheus step value to whole seconds.
// Bare "60" means 60 seconds. Rejects zero, negatives, and malformed input.
func ParseStepSeconds(step string) (int64, error) {
	step = strings.TrimSpace(step)
	if step == "" {
		return 0, fmt.Errorf("step is required")
	}
	if n, err := strconv.ParseInt(step, 10, 64); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("step must be positive")
		}
		return n, nil
	}
	if strings.HasPrefix(step, "-") || !promDuration.MatchString(step) {
		return 0, fmt.Errorf("invalid step %q (use seconds or a Prometheus duration like 30s, 1m)", step)
	 }
	// Sum <number><unit> groups. The regex guarantees full-string match.
	var total int64
	rest := step
	for len(rest) > 0 {
		i := 0
		for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
			i++
		}
		n, _ := strconv.ParseInt(rest[:i], 10, 64)
		rest = rest[i:]
		j := 0
		for j < len(rest) && (rest[j] < '0' || rest[j] > '9') {
			j++
		}
		unit := rest[:j]
		rest = rest[j:]
		var mult int64
		switch unit {
		case "ms":
			// Sub-second steps are meaningless for our ranges; round up.
			mult = 0
		case "s":
			mult = 1
		case "m":
			mult = 60
		case "h":
			mult = 3600
		case "d":
			mult = 86400
		case "w":
			mult = 604800
		case "y":
			mult = 365 * 86400
		}
		if unit == "ms" {
			total++
			continue
		}
		total += n * mult
		// Guard against absurd values before they reach Prometheus.
		if total > MaxRangeSeconds {
			break
		}
	}
	if total <= 0 {
		return 0, fmt.Errorf("step must be positive")
	}
	return total, nil
}

// ValidateStep checks a range-query step against its window: it must parse,
// be at least MinStepSeconds, and not exceed the window itself (a larger
// step collapses the whole range into one point).
func ValidateStep(step string, windowSeconds int64) error {
	secs, err := ParseStepSeconds(step)
	if err != nil {
		return err
	}
	if secs < MinStepSeconds {
		return fmt.Errorf("step must be at least %ds", MinStepSeconds)
	}
	if secs > windowSeconds {
		return fmt.Errorf("step must not exceed the %ds query window", windowSeconds)
	}
	return nil
}

// ValidateWindow checks explicit unix start/end bounds: integers, ordered,
// and no wider than MaxRangeSeconds.
func ValidateWindow(start, end string) (int64, error) {
	s, err := strconv.ParseInt(strings.TrimSpace(start), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("start must be a unix timestamp")
	}
	e, err := strconv.ParseInt(strings.TrimSpace(end), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("end must be a unix timestamp")
	}
	if e <= s {
		return 0, fmt.Errorf("end must be after start")
	}
	if e-s > MaxRangeSeconds {
		return 0, fmt.Errorf("query window must not exceed 7d")
	}
	return e - s, nil
}
