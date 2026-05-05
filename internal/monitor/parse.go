package monitor

import (
	"bufio"
	"io"
	"strings"
	"time"
)

func Parse(alias string, fetchedAt time.Time, r io.Reader) *Snapshot {
	snap := &Snapshot{Alias: alias, FetchedAt: fetchedAt}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "uptime":
			snap.Uptime = value
		case "load":
			snap.Load = value
		case "mem":
			snap.Mem = value
		case "disk":
			snap.Disk = value
		case "topproc":
			snap.TopProcs = value
		case "conns":
			snap.Conns = value
		}
	}
	return snap
}
