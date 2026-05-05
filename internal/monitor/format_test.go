package monitor

import "testing"

func TestParseUptime(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"05:17:09 up 140 days,  3 users,  load average: 0.18, 0.25, 0.21", "140d"},
		{"14:32:01 up 5 days, 3:21, 1 user, load average: 0.42, 0.51, 0.48", "5d"},
		{"22:30 up 1 day, 2:15, 2 users", "1d"},
		{"22:30 up 3:27, 2 users", "3h 27m"},
		{"22:30 up 7 min, 1 user", "7m"},
		{"22:30 up 1 min, 1 user", "1m"},
		{"", ""},
		{"garbage", "garbage"},
	}
	for _, c := range cases {
		if got := ParseUptime(c.in); got != c.want {
			t.Errorf("ParseUptime(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseLoad(t *testing.T) {
	one, five, fifteen, ok := ParseLoad("0.18, 0.25, 0.21")
	if !ok || one != "0.18" || five != "0.25" || fifteen != "0.21" {
		t.Errorf("comma-sep: got (%q,%q,%q,%v)", one, five, fifteen, ok)
	}
	one, five, fifteen, ok = ParseLoad("1.23 1.45 1.67")
	if !ok || one != "1.23" || five != "1.45" || fifteen != "1.67" {
		t.Errorf("space-sep: got (%q,%q,%q,%v)", one, five, fifteen, ok)
	}
	one, _, _, ok = ParseLoad("0.42")
	if !ok || one != "0.42" {
		t.Errorf("single: got (%q,%v)", one, ok)
	}
	if _, _, _, ok = ParseLoad(""); ok {
		t.Error("empty load should return ok=false")
	}
}

func TestParseMemPct(t *testing.T) {
	used, total, unit, pct, ok := ParseMemPct("4451/15944 MB (28%)")
	if !ok || used != 4451 || total != 15944 || unit != "MB" || pct != 28 {
		t.Errorf("nominal: got (%d,%d,%q,%d,%v)", used, total, unit, pct, ok)
	}
	if _, _, _, _, ok := ParseMemPct("n/a"); ok {
		t.Error("n/a should return ok=false")
	}
	used, total, unit, pct, ok = ParseMemPct("12345/16384 MB (75%)")
	if !ok || used != 12345 || total != 16384 || pct != 75 || unit != "MB" {
		t.Errorf("mac-style: got (%d,%d,%q,%d,%v)", used, total, unit, pct, ok)
	}
}

func TestShortMemBytes(t *testing.T) {
	cases := []struct {
		val  int64
		unit string
		want string
	}{
		{4451, "MB", "4.3G"},
		{15944, "MB", "16G"},
		{650, "MB", "650M"},
		{2, "GB", "2.0G"},
		{12, "GB", "12G"},
		{500, "KB", "500K"},
	}
	for _, c := range cases {
		if got := ShortMemBytes(c.val, c.unit); got != c.want {
			t.Errorf("ShortMemBytes(%d,%q) = %q, want %q", c.val, c.unit, got, c.want)
		}
	}
}

func TestParseDiskPct(t *testing.T) {
	used, total, pct, ok := ParseDiskPct("17G used / 61G (28%)")
	if !ok || used != "17G" || total != "61G" || pct != 28 {
		t.Errorf("nominal: got (%q,%q,%d,%v)", used, total, pct, ok)
	}
	used, total, pct, ok = ParseDiskPct("200G used / 500G (40%)")
	if !ok || used != "200G" || total != "500G" || pct != 40 {
		t.Errorf("large: got (%q,%q,%d,%v)", used, total, pct, ok)
	}
	if _, _, _, ok := ParseDiskPct("n/a"); ok {
		t.Error("n/a should return ok=false")
	}
}

func TestParseTopProcs(t *testing.T) {
	procs := ParseTopProcs("systemd(13.8%) azure-proxy-agent(8.2%) containerd(5.1%)")
	if len(procs) != 3 {
		t.Fatalf("want 3 procs, got %d (%+v)", len(procs), procs)
	}
	if procs[0].Name != "systemd" || procs[0].Pct != 13.8 {
		t.Errorf("first proc wrong: %+v", procs[0])
	}
	if procs[1].Name != "azure-proxy-agent" || procs[1].Pct != 8.2 {
		t.Errorf("second proc wrong: %+v", procs[1])
	}

	// Out-of-order input must come back sorted desc.
	procs = ParseTopProcs("a(1.0%) b(50.0%) c(10.0%)")
	if procs[0].Name != "b" || procs[1].Name != "c" || procs[2].Name != "a" {
		t.Errorf("expected b,c,a got %+v", procs)
	}

	if got := ParseTopProcs(""); got != nil {
		t.Errorf("empty input should return nil, got %+v", got)
	}
}

func TestProgressBar(t *testing.T) {
	cases := []struct {
		pct, cells int
		want       string
	}{
		{0, 10, "▱▱▱▱▱▱▱▱▱▱"},
		{1, 10, "▰▱▱▱▱▱▱▱▱▱"}, // tiny pct still gets at least one cell
		{50, 10, "▰▰▰▰▰▱▱▱▱▱"},
		{100, 10, "▰▰▰▰▰▰▰▰▰▰"},
		{28, 10, "▰▰▱▱▱▱▱▱▱▱"},
		{85, 10, "▰▰▰▰▰▰▰▰▱▱"},
		{150, 8, "▰▰▰▰▰▰▰▰"}, // clamp
		{0, 0, ""},
	}
	for _, c := range cases {
		if got := ProgressBar(c.pct, c.cells); got != c.want {
			t.Errorf("ProgressBar(%d,%d) = %q, want %q", c.pct, c.cells, got, c.want)
		}
	}
}

func TestHealthLevel(t *testing.T) {
	cases := []struct {
		pct  int
		want Level
	}{
		{0, LevelOK},
		{59, LevelOK},
		{60, LevelWarn},
		{84, LevelWarn},
		{85, LevelCrit},
		{100, LevelCrit},
	}
	for _, c := range cases {
		if got := HealthLevel(c.pct); got != c.want {
			t.Errorf("HealthLevel(%d) = %d, want %d", c.pct, got, c.want)
		}
	}
}
