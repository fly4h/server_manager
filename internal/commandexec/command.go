package commandexec

import (
	"fmt"
	"regexp"
	"strings"
)

// paramPattern matches {{param}} placeholders in command templates.
var paramPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// allowedParam keeps user-supplied values from injecting shell metacharacters.
var allowedParam = regexp.MustCompile(`^[a-zA-Z0-9._@:%/,-]+$`)

// ExtractParams lists parameter names used by the template.
func ExtractParams(command string) []string {
	seen := map[string]bool{}
	var names []string
	for _, m := range paramPattern.FindAllStringSubmatch(command, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	return names
}

// Render substitutes {{param}} placeholders after validating each value.
// Values failing the whitelist are rejected — never string-spliced blindly.
func Render(command string, params map[string]string) (string, error) {
	var invalid []string
	out := paramPattern.ReplaceAllStringFunc(command, func(match string) string {
		name := strings.TrimSpace(match[2 : len(match)-2])
		val, ok := params[name]
		if !ok || val == "" {
			return match // leave unresolved so the error below fires
		}
		if !allowedParam.MatchString(val) {
			invalid = append(invalid, name)
			return match
		}
		return val
	})
	if invalid != nil {
		return "", fmt.Errorf("parameter %q contains characters not allowed (allowed: letters, digits, dot, underscore, at, colon, percent, slash, comma, dash)", strings.Join(invalid, ", "))
	}
	if leftover := paramPattern.FindString(out); leftover != "" {
		return "", fmt.Errorf("missing value for parameter %s", strings.TrimSpace(leftover[2:len(leftover)-2]))
	}
	return out, nil
}

// DangerLevel returns the effective danger level of a rendered command:
// the max of the stored level and what static analysis of the command says.
func DangerLevel(stored, command string) string {
	level := stored
	analyzed := AnalyzeDanger(command)
	return maxLevel(level, analyzed)
}

var criticalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(^|\s|;|&&|\|)mkfs(\.|\s)`),
	regexp.MustCompile(`(^|\s|;|&&|\|)dd\s+.*of=/dev/`),
	regexp.MustCompile(`\bshutdown\b`),
	regexp.MustCompile(`\breboot\b`),
	regexp.MustCompile(`\bhalt\b`),
	regexp.MustCompile(`\bpoweroff\b`),
	regexp.MustCompile(`>\s*/dev/sd[a-z]`),
}

var highPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(^|\s|;|&&|\|)rm\s+(-[a-zA-Z]*[rf][a-zA-Z]*\s+)+`),
	regexp.MustCompile(`(^|\s|;|&&|\|)rm\s+.*\*`),
	regexp.MustCompile(`\bdocker\s+system\s+prune\b`),
	regexp.MustCompile(`\bdocker\s+(rm|stop|rmi)\s+-[a-zA-Z]*f`),
	regexp.MustCompile(`\biptables\b|\bnftables\b|\bufw\b`),
	regexp.MustCompile(`(^|\s|;|&&|\|)chmod\s+-R?\s*777`),
	regexp.MustCompile(`\bkillall\b|\bpkill\b`),
	regexp.MustCompile(`\bsystemctl\s+(mask|disable)`),
}

var mediumPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bdocker\s+(restart|stop|rm|rmi)\b`),
	regexp.MustCompile(`\bsystemctl\s+(restart|stop|start)\b`),
	regexp.MustCompile(`(^|\s|;|&&|\|)(mv|cp)\s+.*\s+/etc/`),
	regexp.MustCompile(`\bapt(-get)?\s+(remove|purge)\b`),
	regexp.MustCompile(`\btruncate\b|\bshred\b`),
}

// AnalyzeDanger inspects a rendered command and returns low..critical.
func AnalyzeDanger(command string) string {
	for _, p := range criticalPatterns {
		if p.MatchString(command) {
			return "critical"
		}
	}
	for _, p := range highPatterns {
		if p.MatchString(command) {
			return "high"
		}
	}
	for _, p := range mediumPatterns {
		if p.MatchString(command) {
			return "medium"
		}
	}
	return "low"
}

var levelRank = map[string]int{"low": 0, "medium": 1, "high": 2, "critical": 3}

func maxLevel(a, b string) string {
	if levelRank[a] >= levelRank[b] {
		return a
	}
	return b
}

// NeedsConfirm reports whether the level demands a second confirmation.
func NeedsConfirm(level string) bool { return levelRank[level] >= levelRank["high"] }
