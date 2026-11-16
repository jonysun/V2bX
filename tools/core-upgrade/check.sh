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
VERIFY_UPSTREAM=0
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
  --verify-upstream                verify pinned release refs with git ls-remote
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
  if grep -q '^replace github.com/quic-go/qpack => ./third_party/qpack-compat$' "$ROOT/go.mod" && check_file "third_party/qpack-compat/decoder.go"; then
    record OK "qpack compatibility bridge is present"
  else
    record WARN "qpack compatibility bridge is not configured"
  fi
  return "$failed"
}

check_version_pin() {
  local module version
  module="$1"
  version="$2"
  if [[ -z "$version" ]] || ! grep -Fq "${module} ${version}" "$ROOT/go.mod"; then
    record ERROR "go.mod does not pin ${module} to ${version}"
    return 1
  fi
  record OK "go.mod pins ${module} to ${version}"
}

check_source_provenance() {
  local failed=0
  local marker expected label
  for item in "third_party/xray-core/V2BX.md:$(manifest_value TARGET_XRAY_COMMIT):Xray" \
              "third_party/sing-box/V2BX.md:$(manifest_value TARGET_SING_COMMIT):sing-box"; do
    marker="${item%%:*}"
    expected="${item#*:}"
    label="${expected##*:}"
    expected="${expected%:*}"
    if check_file "$marker" && grep -Fq "$expected" "$ROOT/$marker"; then
      record OK "${label} source marker records commit ${expected}"
    else
      record ERROR "${label} source marker does not match pinned commit ${expected}"
      failed=1
    fi
  done

  check_version_pin github.com/apernet/hysteria/core/v2 "$(manifest_value TARGET_HYSTERIA)" || failed=1
  check_version_pin github.com/apernet/hysteria/extras/v2 "$(manifest_value TARGET_HYSTERIA)" || failed=1
  check_version_pin github.com/sagernet/sing-box "$(manifest_value TARGET_SING)" || failed=1
  check_version_pin github.com/xtls/xray-core "$(manifest_value TARGET_XRAY_MODULE_VERSION)" || failed=1
  return "$failed"
}

verify_upstream_ref() {
  local label repo ref expected actual output status detail
  label="$1"
  repo="$2"
  ref="$3"
  expected="$4"
  if output="$(timeout 30s git ls-remote "$repo" "$ref" "${ref}^{}" 2>&1)"; then
    actual="$(printf '%s\n' "$output" | awk -v ref="$ref" '$2 == ref "^{}" { print $1; exit }')"
    if [[ -z "$actual" ]]; then
      actual="$(printf '%s\n' "$output" | awk -v ref="$ref" '$2 == ref { print $1; exit }')"
    fi
  else
    status=$?
    detail="${output:-no diagnostic}"
    if [[ "$status" -eq 124 ]]; then
      detail="timed out after 30 seconds${output:+: $output}"
    fi
    record ERROR "unable to query upstream ${label} ${ref} (git ls-remote exit ${status}): ${detail}"
    return 1
  fi
  if [[ "$actual" == "$expected" ]]; then
    record OK "upstream ${label} ${ref} resolves to ${expected}"
  elif [[ -z "$actual" ]]; then
    record ERROR "upstream ${label} has no ref ${ref}"
    return 1
  else
    record ERROR "upstream ${label} ${ref} resolves to ${actual}, expected ${expected}"
    return 1
  fi
}

check_upstream_refs() {
  local failed=0
  verify_upstream_ref Xray "$(manifest_value TARGET_XRAY_REPO)" "$(manifest_value TARGET_XRAY_REF)" "$(manifest_value TARGET_XRAY_COMMIT)" || failed=1
  verify_upstream_ref sing-box "$(manifest_value TARGET_SING_REPO)" "$(manifest_value TARGET_SING_REF)" "$(manifest_value TARGET_SING_COMMIT)" || failed=1
  verify_upstream_ref Hysteria "$(manifest_value TARGET_HYSTERIA_REPO)" "$(manifest_value TARGET_HYSTERIA_REF)" "$(manifest_value TARGET_HYSTERIA_COMMIT)" || failed=1
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

test_packages_for_core() {
  case "$1" in
    all) printf '%s' './...' ;;
    xray) printf '%s' '. ./cmd ./api/panel ./conf ./common/... ./core ./core/imports ./core/xray/... ./limiter ./node' ;;
    sing) printf '%s' '. ./cmd ./api/panel ./conf ./common/... ./core ./core/imports ./core/sing/... ./limiter ./node' ;;
    hysteria2) printf '%s' '. ./cmd ./api/panel ./conf ./common/... ./core ./core/imports ./core/hy2/... ./limiter ./node' ;;
    *) return 1 ;;
  esac
}

behavior_test_packages_for_core() {
  case "$1" in
    all) printf '%s' '. ./api/panel ./common/... ./core ./core/imports ./core/xray/... ./core/sing/... ./core/hy2/... ./limiter ./node' ;;
    xray) printf '%s' '. ./api/panel ./common/... ./core ./core/imports ./core/xray/... ./limiter ./node' ;;
    sing) printf '%s' '. ./api/panel ./common/... ./core ./core/imports ./core/sing/... ./limiter ./node' ;;
    hysteria2) printf '%s' '. ./api/panel ./common/... ./core ./core/imports ./core/hy2/... ./limiter ./node' ;;
    *) return 1 ;;
  esac
}

run_docker_build() {
  local tags="$1"
  local packages_string="$2"
  local packages=()
  read -r -a packages <<< "$packages_string"
  docker_go go version || return $?
  # Compile every package without running external or networked tests. The
  # full behavior suite runs as a separate release gate after this probe.
  docker_go go test "${packages[@]}" -run '^$' -tags "$tags" || return $?
  docker_go env GOEXPERIMENT=jsonv2 go build -trimpath -o /tmp/V2bX -tags "$tags" || return $?
}

run_docker_behavior_tests() {
  local tags="$1"
  local packages_string="$2"
  local packages=()
  read -r -a packages <<< "$packages_string"
  docker_go go test "${packages[@]}" -count=1 -tags "$tags" || return $?
  # TestConf_Watch intentionally blocks forever; run only the bounded config-load test.
  docker_go go test ./conf -run '^TestConf_LoadFromPath$' -count=1 -tags "$tags"
}

run_xray_fork_tests() {
  local quic_version
  quic_version="$(manifest_value TARGET_QUIC)"
  docker_go sh -c "cd /src/third_party/xray-core && cp go.mod /tmp/xray-test.mod && cp go.sum /tmp/xray-test.sum && go mod edit -modfile=/tmp/xray-test.mod -require=${quic_version} && go mod edit -modfile=/tmp/xray-test.mod -replace=github.com/quic-go/qpack=/src/third_party/qpack-compat && go test -mod=mod -modfile=/tmp/xray-test.mod ./proxy/shadowsocks && go test -mod=mod -modfile=/tmp/xray-test.mod ./infra/conf -run 'TestShadowsocksServerConfigParsing|TestShadowsocksIVCheckConfigCompatibility'"
}

run_sing_box_fork_tests() {
  local quic_version
  quic_version="$(manifest_value TARGET_QUIC)"
  docker_go sh -c "cd /src/third_party/sing-box && cp go.mod /tmp/sing-test.mod && cp go.sum /tmp/sing-test.sum && go mod edit -modfile=/tmp/sing-test.mod -require=${quic_version} && go mod edit -modfile=/tmp/sing-test.mod -replace=github.com/quic-go/qpack=/src/third_party/qpack-compat && go test -mod=mod -modfile=/tmp/sing-test.mod ./adapter ./adapter/inbound ./protocol/v2bxusers"
}

run_source_snapshot_check() {
  local failed=0
  if [[ "$CORE" == "xray" || "$CORE" == "all" ]]; then
    docker_go bash tools/core-upgrade/check-source-snapshot.sh \
      github.com/xtls/xray-core \
      "$(manifest_value TARGET_XRAY_MODULE_VERSION)" \
      "$(manifest_value TARGET_XRAY_COMMIT)" \
      third_party/xray-core tools/core-upgrade/xray-patch-files.txt || failed=1
  fi
  if [[ "$CORE" == "sing" || "$CORE" == "all" ]]; then
    docker_go bash tools/core-upgrade/check-source-snapshot.sh \
      github.com/sagernet/sing-box \
      "$(manifest_value TARGET_SING)" \
      "$(manifest_value TARGET_SING_COMMIT)" \
      third_party/sing-box tools/core-upgrade/sing-box-patch-files.txt || failed=1
  fi
  return "$failed"
}

run_build_check() {
  local tags="$1"
  local packages_string="$2"
  local behavior_packages
  if ! docker_available; then
    record ERROR "Docker daemon is unavailable; use --skip-build for static checks only"
    return 1
  fi
  record OK "Docker daemon is available"
  if [[ "$CORE" != "hysteria2" ]]; then
    if run_source_snapshot_check >"$RUN_TMPDIR/v2bx-core-sources.out" 2>&1; then
      record OK "upstream source snapshots match their V2bX patch allowlists (${CORE})"
    else
      record ERROR "upstream source snapshot check (${CORE}) failed\n$(sed -n '1,100p' "$RUN_TMPDIR/v2bx-core-sources.out")"
      return 1
    fi
  fi
  if run_docker_build "$tags" "$packages_string" >"$RUN_TMPDIR/v2bx-core-check.out" 2>&1; then
    record OK "Docker compile/test (${CORE})"
  else
    record ERROR "Docker compile/test (${CORE}) failed\n$(sed -n '1,100p' "$RUN_TMPDIR/v2bx-core-check.out")"
    return 1
  fi

  behavior_packages="$(behavior_test_packages_for_core "$CORE")"
  if run_docker_behavior_tests "$tags" "$behavior_packages" >"$RUN_TMPDIR/v2bx-core-tests.out" 2>&1; then
    record OK "Docker behavior tests (${CORE})"
  else
    record ERROR "Docker behavior tests (${CORE}) failed\n$(sed -n '1,100p' "$RUN_TMPDIR/v2bx-core-tests.out")"
    return 1
  fi

  if [[ "$CORE" == "xray" || "$CORE" == "all" ]]; then
    if run_xray_fork_tests >"$RUN_TMPDIR/v2bx-xray-fork-tests.out" 2>&1; then
      record OK "Xray fork compatibility tests"
    else
      record ERROR "Xray fork compatibility tests failed\n$(sed -n '1,100p' "$RUN_TMPDIR/v2bx-xray-fork-tests.out")"
      return 1
    fi
  fi
  if [[ "$CORE" == "sing" || "$CORE" == "all" ]]; then
    if run_sing_box_fork_tests >"$RUN_TMPDIR/v2bx-sing-box-fork-tests.out" 2>&1; then
      record OK "sing-box fork compatibility tests"
    else
      record ERROR "sing-box fork compatibility tests failed\n$(sed -n '1,100p' "$RUN_TMPDIR/v2bx-sing-box-fork-tests.out")"
      return 1
    fi
  fi
  return 0
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode) MODE="${2:-}"; shift 2 ;;
    --core) CORE="${2:-}"; shift 2 ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --allow-dirty) ALLOW_DIRTY=1; shift ;;
    --verify-upstream) VERIFY_UPSTREAM=1; shift ;;
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
	REPORT="$ROOT/tools/core-upgrade/reports/core-compat-${CORE}-${MODE}-$(date -u +%Y%m%dT%H%M%SZ)-$$.md"
fi
mkdir -p "$(dirname "$REPORT")"
RUN_TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/v2bx-core-check.XXXXXX")"
trap 'rm -rf "$RUN_TMPDIR"' EXIT
{
  printf '%s\n\n' '# V2bX Core Compatibility Report'
  printf '%s\n' "- Mode: \`${MODE}\`"
  printf '%s\n' "- Core: \`${CORE}\`"
  printf '%s\n' "- Target Xray: \`$(manifest_value TARGET_XRAY)\`"
  printf '%s\n' "- Xray commit: \`$(manifest_value TARGET_XRAY_COMMIT)\`"
  printf '%s\n' "- Target sing-box: \`$(manifest_value TARGET_SING)\`"
  printf '%s\n' "- sing-box commit: \`$(manifest_value TARGET_SING_COMMIT)\`"
  printf '%s\n' "- Target Hysteria: \`$(manifest_value TARGET_HYSTERIA)\`"
  printf '%s\n' "- Hysteria commit: \`$(manifest_value TARGET_HYSTERIA_COMMIT)\`"
  printf '%s\n\n' "- Started: \`$(date -u +%Y-%m-%dT%H:%M:%SZ)\`"
  printf '%s\n' '## Checks'
} > "$REPORT"

check_git_state || true
check_source_layout || true
check_build_tags || true
check_dependencies || true
check_source_provenance || true
check_go_versions || true
if [[ "$VERIFY_UPSTREAM" -eq 1 ]]; then
  check_upstream_refs || true
fi

if [[ "$SKIP_BUILD" -eq 1 ]]; then
  record WARN "Docker build was skipped by --skip-build"
else
  tags="$(build_tags_for_core "$CORE")"
  packages_string="$(test_packages_for_core "$CORE")"
  run_build_check "$tags" "$packages_string" || true
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
