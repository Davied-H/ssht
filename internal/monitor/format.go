package monitor

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Level grades a percentage into a health bucket.
type Level int

const (
	LevelOK Level = iota
	LevelWarn
	LevelCrit
)

const (
	healthWarnPct = 60
	healthCritPct = 85
)

// HealthLevel maps a percentage (0-100) to OK/Warn/Crit using fixed thresholds.
func HealthLevel(pct int) Level {
	switch {
	case pct >= healthCritPct:
		return LevelCrit
	case pct >= healthWarnPct:
		return LevelWarn
	default:
		return LevelOK
	}
}

// ProcInfo is one row of the top-CPU list.
type ProcInfo struct {
	Name string
	Pct  float64
}

var (
	uptimeDaysRe    = regexp.MustCompile(`up\s+(\d+)\s+days?`)
	uptimeHoursRe   = regexp.MustCompile(`up\s+(\d+):(\d+)`)
	uptimeMinsRe    = regexp.MustCompile(`up\s+(\d+)\s+min`)
	uptimeMixedRe   = regexp.MustCompile(`up\s+(\d+)\s+days?,\s+(\d+):(\d+)`)
	loadNumRe       = regexp.MustCompile(`-?\d+(?:\.\d+)?`)
	memRe           = regexp.MustCompile(`(\d+)\s*/\s*(\d+)\s*([KMGT]?B)\s*\((\d+)\s*%\)`)
	diskRe          = regexp.MustCompile(`(\S+)\s+used\s*/\s*(\S+)\s*\((\d+)\s*%\)`)
	procRe          = regexp.MustCompile(`(\S+?)\(([0-9.]+)%?\)`)
)

// ParseUptime extracts a compact human form from the raw uptime line returned by
// the probe (e.g. "05:17:09 up 140 days, 3 users, load average:..."). Returns
// the original string when no recognisable shape is found.
func ParseUptime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if m := uptimeMixedRe.FindStringSubmatch(s); m != nil {
		days, _ := strconv.Atoi(m[1])
		if days > 0 {
			return fmt.Sprintf("%dd", days)
		}
	}
	if m := uptimeDaysRe.FindStringSubmatch(s); m != nil {
		days, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("%dd", days)
	}
	if m := uptimeHoursRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.Atoi(m[1])
		min, _ := strconv.Atoi(m[2])
		return fmt.Sprintf("%dh %02dm", h, min)
	}
	if m := uptimeMinsRe.FindStringSubmatch(s); m != nil {
		min, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("%dm", min)
	}
	// Fall back to "up X" without the leading clock when present.
	if idx := strings.Index(s, " up "); idx >= 0 {
		rest := strings.TrimSpace(s[idx+4:])
		if comma := strings.Index(rest, ","); comma > 0 {
			return rest[:comma]
		}
		return rest
	}
	return s
}

// ParseLoad extracts up to three load numbers (1m, 5m, 15m) from a raw load
// string. Linux delivers them comma-separated; BSD/mac space-separated. Missing
// fields come back as empty strings.
func ParseLoad(s string) (one, five, fifteen string, ok bool) {
	matches := loadNumRe.FindAllString(s, -1)
	if len(matches) == 0 {
		return "", "", "", false
	}
	if len(matches) > 0 {
		one = matches[0]
	}
	if len(matches) > 1 {
		five = matches[1]
	}
	if len(matches) > 2 {
		fifteen = matches[2]
	}
	return one, five, fifteen, true
}

// ParseMemPct parses "<used>/<total> <UNIT> (<pct>%)" into structured values.
// Returns ok=false when the input is missing or doesn't match.
func ParseMemPct(s string) (used, total int64, unit string, pct int, ok bool) {
	m := memRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, "", 0, false
	}
	used, _ = strconv.ParseInt(m[1], 10, 64)
	total, _ = strconv.ParseInt(m[2], 10, 64)
	unit = m[3]
	pct, _ = strconv.Atoi(m[4])
	return used, total, unit, pct, true
}

// ShortMemBytes turns a (value, unit) pair like (4451, "MB") into a compact
// "4.3G" / "650M" form. unit is one of KB/MB/GB/TB; falls back to printing the
// raw value when unrecognised.
func ShortMemBytes(value int64, unit string) string {
	scale := int64(1)
	switch strings.ToUpper(unit) {
	case "KB":
		scale = 1
	case "MB":
		scale = 1024
	case "GB":
		scale = 1024 * 1024
	case "TB":
		scale = 1024 * 1024 * 1024
	default:
		return fmt.Sprintf("%d%s", value, unit)
	}
	asKB := value * scale
	switch {
	case asKB >= 1024*1024*1024:
		return fmt.Sprintf("%.1fT", float64(asKB)/float64(1024*1024*1024))
	case asKB >= 1024*1024:
		v := float64(asKB) / float64(1024*1024)
		if v >= 10 {
			return fmt.Sprintf("%.0fG", v)
		}
		return fmt.Sprintf("%.1fG", v)
	case asKB >= 1024:
		return fmt.Sprintf("%dM", asKB/1024)
	default:
		return fmt.Sprintf("%dK", asKB)
	}
}

// ParseDiskPct parses "<used> used / <total> (<pct>%)" into structured values.
func ParseDiskPct(s string) (used, total string, pct int, ok bool) {
	m := diskRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", 0, false
	}
	used = m[1]
	total = m[2]
	pct, _ = strconv.Atoi(m[3])
	return used, total, pct, true
}

// ParseTopProcs splits the raw "name(pct%) name(pct%)" string into structured
// rows, sorted descending by percentage.
func ParseTopProcs(s string) []ProcInfo {
	matches := procRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil
	}
	procs := make([]ProcInfo, 0, len(matches))
	for _, m := range matches {
		pct, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			continue
		}
		procs = append(procs, ProcInfo{Name: m[1], Pct: pct})
	}
	sort.SliceStable(procs, func(i, j int) bool { return procs[i].Pct > procs[j].Pct })
	return procs
}

// ProgressBar renders an n-cell horizontal bar where the first ceil(pct/100*n)
// cells are filled. pct is clamped to 0..100; cells must be > 0.
func ProgressBar(pct, cells int) string {
	if cells <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * cells / 100
	if pct > 0 && filled == 0 {
		filled = 1
	}
	if filled > cells {
		filled = cells
	}
	var b strings.Builder
	b.Grow(cells * 3)
	for i := 0; i < cells; i++ {
		if i < filled {
			b.WriteRune('▰')
		} else {
			b.WriteRune('▱')
		}
	}
	return b.String()
}
