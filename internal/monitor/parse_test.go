package monitor

import (
	"strings"
	"testing"
	"time"
)

func TestParseLinuxStyle(t *testing.T) {
	input := `uptime: 14:32:01 up 5 days, 3:21, 1 user, load average: 0.42, 0.51, 0.48
load: 0.42, 0.51, 0.48
mem: 2143/7820 MB (27%)
disk: 14G used / 40G (37%)
topproc: nginx(8.2%) python3(3.1%) go(1.5%)
conns: 143
`
	snap := Parse("box", time.Unix(1, 0), strings.NewReader(input))
	if snap.Alias != "box" {
		t.Fatalf("alias=%q", snap.Alias)
	}
	if snap.Uptime != "14:32:01 up 5 days, 3:21, 1 user, load average: 0.42, 0.51, 0.48" {
		t.Fatalf("uptime=%q", snap.Uptime)
	}
	if snap.Load != "0.42, 0.51, 0.48" {
		t.Fatalf("load=%q", snap.Load)
	}
	if snap.Mem != "2143/7820 MB (27%)" {
		t.Fatalf("mem=%q", snap.Mem)
	}
	if snap.Disk != "14G used / 40G (37%)" {
		t.Fatalf("disk=%q", snap.Disk)
	}
	if snap.TopProcs != "nginx(8.2%) python3(3.1%) go(1.5%)" {
		t.Fatalf("topproc=%q", snap.TopProcs)
	}
	if snap.Conns != "143" {
		t.Fatalf("conns=%q", snap.Conns)
	}
}

func TestParseMacStyle(t *testing.T) {
	input := `uptime: 14:32  up 5 days, 12:34, 3 users, load averages: 1.23 1.45 1.67
load: 1.23 1.45 1.67
mem: 12345/16384 MB (75%)
disk: 200G used / 500G (40%)
topproc: WindowServer(15%) Chrome(8%)
conns: 47
`
	snap := Parse("mac", time.Unix(1, 0), strings.NewReader(input))
	if snap.Load != "1.23 1.45 1.67" {
		t.Fatalf("load=%q", snap.Load)
	}
	if snap.Mem != "12345/16384 MB (75%)" {
		t.Fatalf("mem=%q", snap.Mem)
	}
	if snap.Conns != "47" {
		t.Fatalf("conns=%q", snap.Conns)
	}
}

func TestParseTolerantOfMissingAndExtra(t *testing.T) {
	input := `garbage line
mem: 100/200 MB (50%)
unknown: ignored
disk: 5G used / 10G (50%)
`
	snap := Parse("x", time.Unix(0, 0), strings.NewReader(input))
	if snap.Mem != "100/200 MB (50%)" || snap.Disk != "5G used / 10G (50%)" {
		t.Fatalf("snap=%+v", snap)
	}
	if snap.Uptime != "" || snap.Load != "" || snap.TopProcs != "" || snap.Conns != "" {
		t.Fatalf("expected empty for missing fields, got %+v", snap)
	}
}
