#!/usr/bin/env sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

failures=0

section() {
  printf '\n==> %s\n' "$1"
}

fail() {
  failures=$((failures + 1))
  printf 'ERROR: %s\n' "$*" >&2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "$1 is required"
    return 1
  fi
  return 0
}

run_go_tests() {
  section "Go tests"
  go_cache="${GOCACHE:-${TMPDIR:-/tmp}/ssht-go-build-cache}"
  mkdir -p "$go_cache"
  GOCACHE="$go_cache" go test ./...
}

scan_sensitive_patterns() {
  section "Sensitive information scan"

  tmp_file=$(mktemp)
  trap 'rm -f "$tmp_file"' EXIT INT TERM

  secret_pattern='BEGIN (RSA |DSA |EC |OPENSSH |)PRIVATE KEY|github_pat_[A-Za-z0-9_]+|gh[opsu]_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{20,}|sk-[A-Za-z0-9]{20,}'
  if git grep -nIE "$secret_pattern" -- . >"$tmp_file" 2>/dev/null; then
    fail "possible private key or token found"
    cat "$tmp_file" >&2
  fi

  if git grep -nE '/Users/[^[:space:]`"<>)]+' -- . | grep -v '^scripts/preflight-release.sh:' >"$tmp_file" 2>/dev/null; then
    if grep -v '/Users/example' "$tmp_file" >"$tmp_file.filtered"; then
      fail "possible personal absolute path found"
      cat "$tmp_file.filtered" >&2
    fi
    rm -f "$tmp_file.filtered"
  fi

  printf 'sensitive scan complete\n'
}

check_document_examples() {
  section "README and usage example compliance"

  docs="README.md docs/usage.md"

  for doc in $docs; do
    if ! [ -f "$doc" ]; then
      fail "$doc is missing"
      continue
    fi

    awk '
      function allowed_ip(ip, parts) {
        split(ip, parts, ".")
        return (parts[1] == 192 && parts[2] == 0 && parts[3] == 2) ||
          (parts[1] == 198 && parts[2] == 51 && parts[3] == 100) ||
          (parts[1] == 203 && parts[2] == 0 && parts[3] == 113)
      }
      {
        line = $0
        while (match(line, /([0-9]{1,3}\.){3}[0-9]{1,3}/)) {
          ip = substr(line, RSTART, RLENGTH)
          if (!allowed_ip(ip)) {
            printf "%s:%d: non-documentation IPv4 example: %s\n", FILENAME, FNR, ip
            bad = 1
          }
          line = substr(line, RSTART + RLENGTH)
        }
      }
      END { exit bad ? 1 : 0 }
    ' "$doc" || fail "$doc contains IPv4 examples outside RFC 5737 ranges"

    awk '
      /^[[:space:]]+HostName[[:space:]]+/ {
        host = $2
        if (host ~ /^192\.0\.2\./ || host ~ /^198\.51\.100\./ || host ~ /^203\.0\.113\./ || host == "example.com" || host ~ /\.example\.com$/) {
          next
        }
        printf "%s:%d: HostName example should use RFC 5737 IPs or example.com: %s\n", FILENAME, FNR, host
        bad = 1
      }
      END { exit bad ? 1 : 0 }
    ' "$doc" || fail "$doc contains non-placeholder HostName examples"

    if grep -nE '/Users/[^[:space:]`"<>)]+' "$doc" | grep -v '/Users/example' >&2; then
      fail "$doc contains a personal absolute path example"
    fi
  done

  printf 'documentation examples are compliant\n'
}

check_release_artifact_names() {
  section "Release artifact naming self-check"

  workflow=".github/workflows/release.yml"
  usage="docs/usage.md"
  installer="scripts/install-release.sh"

  for file in "$workflow" "$usage" "$installer"; do
    if ! [ -f "$file" ]; then
      fail "$file is missing"
    fi
  done

  while read -r goos goarch ext; do
    [ -n "$goos" ] || continue

    if ! grep -Fq "goos: $goos" "$workflow"; then
      fail "release workflow is missing GOOS $goos"
    fi
    if ! grep -Fq "goarch: $goarch" "$workflow"; then
      fail "release workflow is missing GOARCH $goarch"
    fi

    doc_name="ssht_<version>_${goos}_${goarch}.${ext}"
    if ! grep -Fq "$doc_name" "$usage"; then
      fail "usage docs are missing release asset $doc_name"
    fi
  done <<'EOF'
darwin amd64 tar.gz
darwin arm64 tar.gz
linux amd64 tar.gz
linux arm64 tar.gz
windows amd64 zip
EOF

  if ! grep -Fq 'build/ssht_${VERSION}_${GOOS}_${GOARCH}' "$workflow"; then
    fail "release workflow no longer builds versioned package directories"
  fi
  if ! grep -Fq 'ssht_${VERSION}_${GOOS}_${GOARCH}.tar.gz' "$workflow"; then
    fail "release workflow tar.gz archive pattern changed"
  fi
  if ! grep -Fq 'ssht_${VERSION}_${GOOS}_${GOARCH}.zip' "$workflow"; then
    fail "release workflow zip archive pattern changed"
  fi
  if ! grep -Fq -- '-X main.version=${VERSION}' "$workflow"; then
    fail "release workflow does not inject the release version into the binary"
  fi
  if ! grep -Fq 'DISPATCH_VERSION: ${{ inputs.version }}' "$workflow"; then
    fail "release workflow does not pass manual versions through the environment"
  fi
  if ! grep -Fq 'Invalid release version' "$workflow"; then
    fail "release workflow does not validate release versions"
  fi
  if ! grep -Fq 'ssht_${VERSION}_${goos}_${goarch}.tar.gz' "$installer"; then
    fail "install-release.sh does not resolve versioned tar.gz asset names"
  fi
  if ! grep -Fq 'checksums.txt' "$workflow" || ! grep -Fq 'checksums.txt' "$installer"; then
    fail "release checksum generation or installer verification is missing"
  fi

  printf 'release naming rules are aligned\n'
}

if require_cmd go && require_cmd git && require_cmd grep && require_cmd awk; then
  run_go_tests
  scan_sensitive_patterns
  check_document_examples
  check_release_artifact_names
fi

if [ "$failures" -ne 0 ]; then
  printf '\nPreflight failed with %s issue(s).\n' "$failures" >&2
  exit 1
fi

printf '\nPreflight passed.\n'
