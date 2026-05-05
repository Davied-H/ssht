package monitor

// probeScript is piped to the remote `sh -s` over SSH stdin. It targets POSIX sh
// (no bashisms) and tolerates Linux, macOS and *BSD by branching on /proc and vm_stat.
// Output is one `key: value` line per metric; the parser ignores unknown keys.
const probeScript = `LC_ALL=C
uptime_line=$(uptime 2>/dev/null)
echo "uptime: $uptime_line"
load=$(printf '%s' "$uptime_line" | sed -n 's/.*load average[s]*: //p')
echo "load: $load"

if [ -r /proc/meminfo ]; then
  awk '/MemTotal:/{t=$2} /MemAvailable:/{a=$2} END{
    if(t>0){used=t-a; printf "mem: %d/%d MB (%.0f%%)\n", used/1024, t/1024, used*100/t}
    else{print "mem: n/a"}
  }' /proc/meminfo
elif command -v vm_stat >/dev/null 2>&1; then
  vm_stat 2>/dev/null | awk '
    /page size of/ {ps=$8}
    /Pages free:/ {free=$3}
    /Pages active:/ {act=$3}
    /Pages wired down:/ {wire=$4}
    END {
      gsub(/\./,"",free); gsub(/\./,"",act); gsub(/\./,"",wire)
      total=(free+act+wire)*ps/1024/1024
      used=(act+wire)*ps/1024/1024
      if(total>0){printf "mem: %.0f/%.0f MB (%.0f%%)\n", used, total, used*100/total}
      else{print "mem: n/a"}
    }'
else
  echo "mem: n/a"
fi

df -h / 2>/dev/null | awk 'NR==2{printf "disk: %s used / %s (%s)\n",$3,$2,$5}'

top=$(ps -eo pcpu,comm 2>/dev/null | tail -n +2 | sort -rn | awk 'NR<=5 && $2!="ps"{printf "%s(%s%%) ", $2, $1}')
echo "topproc: $top"

if command -v ss >/dev/null 2>&1; then
  echo "conns: $(ss -tnH state established 2>/dev/null | wc -l | tr -d ' ')"
elif command -v netstat >/dev/null 2>&1; then
  echo "conns: $(netstat -an 2>/dev/null | grep -c ESTABLISHED)"
else
  echo "conns: n/a"
fi
`
