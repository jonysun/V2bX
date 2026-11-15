#!/usr/bin/env bash
set -u -o pipefail

# Pre-update checks are read-only. Release mode adds Docker builds, but never
# edits go.mod, go.sum, or source files.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MANIFEST="$SCRIPT_DIR/versions.env"
MODE="pre-update"
CORE="all"
SKIP_BUILD=0
ALLOW_DIRTY=0
REPORT=""
FAILURES=0
WARNINGS=0

usage() {
  cat <<'EOF'
Usage: tools/core-upgrade/check.sh [options]

Options:
  --mode pre-update|release       pre-update is static; release also builds in Docker
  --core all|xray|sing|hysteria2  build one core or the full release combination
  --skip-build                    skip Docker checks and compilation
  --allow-dirty                   allow unrelated working-tree changes
  --report PATH                   write the Markdown report to PATH
  -h, --help                      show this help
EOF
}

manifest_value() {
  local key="$1"
  sed -n "s/^${key}=//p" "$MANIFEST" | head -n 1
}

record() {
  local level="$1"
  shift
  local message="$*"
  printf '[%s] %s\n' "$level" "$message"
  if [[ -n "$REPORT" ]]; then
    printf -- '- **%s:** %s\n' "$level" "$message" >> "$REPORT"
  fi
  case "$level" in
    ERROR) FAILURES=$((FAILURES + 1)) ;;
    WARN) WARNINGS=$((WARNINGS + 1)) ;;
  esac
}

version_ge() {
  local left="${1#v}"
  local right="${2#v}"
  [[ "$(printf '%s\n%s\n' "$left" "$right" | sort -V | tail -n 1)" == "$left" ]]
}

check_file() {
  [[ -f "$ROOT/$1" ]]
}

check_git_state() {
  local branch dirty
  branch="$(git -C "$ROOT" branch --show-current 2>/dev/null || true)"
  if [[ -z "$branch" ]]; then
    record ERROR "not inside a Git worktree"
    return 1
  fi
  record OK "Git branch: ${branch}"
  dirty="$(git -C "$ROOT" status --short 2>/dev/null || true)"
  if [[ -n "$dirty" && "$ALLOW_DIRTY" -eq 0 ]]; then
    record ERROR "working tree is dirty; use --allow-dirty only for an intentional local check"
    return 1
  elif [[ -n "$dirty" ]]; then
    record WARN "working tree has local changes and was allowed"
  else
    record OK "working tree is clean"
  fi
}

check_source_layout() {
  local failed=0
  local path
  for path in core/interface.go core/selector.go core/xray/xray.go core/sing/sing.go core/hy2/hy2.go Dockerfile; do
    if check_file "$path"; then
      record OK "required path exists: ${path}"
    else
      record ERROR "required path is missing: ${path}"
      failed=1
    fi
  done
  return "$failed"
}

check_build_tags() {
  local failed=0
  local item tag path
  for item in "xray:core/imports/xray.go" "sing:core/imports/sing.go" "hysteria2:core/imports/hy2.go"; do
    tag="${item%%:*}"
    path="${item#*:}"
    if ! check_file "$path" || ! grep -q "^//go:build ${tag}$" "$ROOT/$path"; then
      record ERROR "missing build tag ${tag} in ${path}"
      failed=1
    else
      record OK "build tag ${tag} is present"
    fi
  done
  return "$failed"
}

check_dependencies() {
  local failed=0
  local module
  for module in github.com/apernet/hysteria/core/v2 github.com/apernet/hysteria/extras/v2 github.com/sagernet/sing-box github.com/xtls/xray-core; do
    if grep -Eq "^[[:space:]]+${module}[[:space:]]" "$ROOT/go.mod"; then
      record OK "go.mod declares ${module}"
    else
      record ERROR "go.mod does not declare ${module}"
      failed=1
    fi
  done
  for module in "github.com/sagernet/sing-box" "github.com/xtls/xray-core"; do
    if grep -q "^replace ${module} " "$ROOT/go.mod"; then
      record OK "fork replace is recorded for ${module}"
    else
      record WARN "no fork replace is recorded for ${module}; official API compatibility must be proven"
    fi
  done
  return "$failed"
}

check_go_versions() {
  local go_version toolchain
  go_version="$(sed -nE 's/^go[[:space:]]+([0-9.]+).*/\1/p' "$ROOT/go.mod" | head -n 1)"
  toolchain="$(sed -nE 's/^toolchain[[:space:]]+go([0-9.]+).*/\1/p' "$ROOT/go.mod" | head -n 1)"
  if [[ -z "$go_version" ]]; then
    record ERROR "go.mod has no Go version"
  elif version_ge "$go_version" "$(manifest_value MIN_GO)"; then
    record OK "go.mod Go version ${go_version} meets minimum $(manifest_value MIN_GO)"
  else
    record ERROR "go.mod Go version ${go_version} is below required $(manifest_value MIN_GO)"
  fi
  if [[ -z "$toolchain" ]]; then
    record WARN "go.mod has no toolchain directive"
  elif version_ge "$toolchain" "$(manifest_value MIN_GO)"; then
    record OK "toolchain go${toolchain} meets minimum $(manifest_value MIN_GO)"
  else
    record ERROR "toolchain go${toolchain} is below required $(manifest_value MIN_GO)"
  fi
}

docker_available() {
  command -v docker >/dev/null 2>&1 || return 1
  timeout 20s docker version --format '{{.Server.Version}}' >/dev/null 2>&1
}

docker_go() {
  local image host_root mod_cache build_cache
  image="$(manifest_value GO_IMAGE)"
  host_root="$ROOT"
  mod_cache="${V2BX_GO_MOD_CACHE:-$ROOT/.cache/go-mod}"
  build_cache="${V2BX_GO_BUILD_CACHE:-$ROOT/.cache/go-build}"
  mkdir -p "$mod_cache" "$build_cache"
  if command -v cygpath >/dev/null 2>&1; then
    host_root="$(cygpath -w "$ROOT")"
    mod_cache="$(cygpath -w "$mod_cache")"
    build_cache="$(cygpath -w "$build_cache")"
  fi
  MSYS_NO_PATHCONV=1 docker run --rm \
    -v "${host_root}:/src" \
    -v "${mod_cache}:/go/pkg/mod" \
    -v "${build_cache}:/root/.cache/go-build" \
    -w /src \
    -e CGO_ENABLED=0 \
    -e GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" \
    "$image" "$@"
}

build_tags_for_core() {
  case "$1" in
    all) printf '%s' "$(manifest_value FULL_TAGS)" ;;
    xray) printf '%s' "$(manifest_value CORE_TAG_XRAY)" ;;
    sing) printf '%s' "$(manifest_value CORE_TAG_SING)" ;;
    hysteria2) printf '%s' "$(manifest_value CORE_TAG_HYSTERIA2)" ;;
    *) return 1 ;;
  esac
}

run_docker_build() {
  local tags="$1"
  docker_go go version
  docker_go go mod verify
  docker_go go test ./... -tags "$tags"
  docker_go env GOEXPERIMENT=jsonv2 go build -trimpath -o /tmp/V2bX -tags "$tags"
}

run_build_check() {
  local tags="$1"
  if ! docker_available; then
    record ERROR "Docker daemon is unavailable; use --skip-build for static checks only"
    return 1
  fi
  record OK "Docker daemon is available"
  if run_docker_build "$tags" >"$TMPDIR/v2bx-core-check.out" 2>&1; then
    record OK "Docker compile/test (${CORE})"
    return 0
  fi
  record ERROR "Docker compile/test (${CORE}) failed\n$(sed -n '1,100p' "$TMPDIR/v2bx-core-check.out")"
  return 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="${2:-}"; shift 2 ;;
    --core) CORE="${2:-}"; shift 2 ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --allow-dirty) ALLOW_DIRTY=1; shift ;;
    --report) REPORT="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

if [[ "$MODE" != "pre-update" && "$MODE" != "release" ]]; then
  printf 'Invalid mode: %s\n' "$MODE" >&2
  exit 2
fi
if [[ "$CORE" != "all" && "$CORE" != "xray" && "$CORE" != "sing" && "$CORE" != "hysteria2" ]]; then
  printf 'Invalid core: %s\n' "$CORE" >&2
  exit 2
fi

if [[ -z "$REPORT" ]]; then
  REPORT="$ROOT/tools/core-upgrade/reports/core-compat-$(date -u +%Y%m%dT%H%M%SZ).md"
fi
mkdir -p "$(dirname "$REPORT")"
{
  printf '%s\n\n' '# V2bX Core Compatibility Report'
  printf '%s\n' "- Mode: \`${MODE}\`"
  printf '%s\n' "- Core: \`${CORE}\`"
  printf '%s\n' "- Target Xray: \`$(manifest_value TARGET_XRAY)\`"
  printf '%s\n' "- Target sing-box: \`$(manifest_value TARGET_SING)\`"
  printf '%s\n' "- Target Hysteria: \`$(manifest_value TARGET_HYSTERIA)\`"
  printf '%s\n\n' "- Started: \`$(date -u +%Y-%m-%dT%H:%M:%SZ)\`"
  printf '%s\n' '## Checks'
} > "$REPORT"

TMPDIR="${TMPDIR:-/tmp}"
check_git_state || true
check_source_layout || true
check_build_tags || true
check_dependencies || true
check_go_versions || true

if [[ "$SKIP_BUILD" -eq 1 ]]; then
  record WARN "Docker build was skipped by --skip-build"
else
  tags="$(build_tags_for_core "$CORE")"
  run_build_check "$tags" || true
fi

{
  printf '\n%s\n\n' '## Result'
  printf '%s\n' "- Errors: ${FAILURES}"
  printf '%s\n' "- Warnings: ${WARNINGS}"
  printf '%s\n' "- Finished: \`$(date -u +%Y-%m-%dT%H:%M:%SZ)\`"
} >> "$REPORT"

printf '\nReport: %s\n' "$REPORT"
if [[ "$FAILURES" -gt 0 ]]; then
  printf 'Compatibility check failed with %s error(s).\n' "$FAILURES"
  exit 1
fi
printf 'Compatibility check passed with %s warning(s).\n' "$WARNINGS"
