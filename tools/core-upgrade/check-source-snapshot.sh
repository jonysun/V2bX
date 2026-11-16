#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 5 ]]; then
  printf 'Usage: %s MODULE VERSION COMMIT SNAPSHOT PATCH_ALLOWLIST\n' "$0" >&2
  exit 2
fi

module="$1"
version="$2"
commit="$3"
snapshot="${4#./}"
allowlist="${5#./}"
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

if [[ ! -d "$snapshot" || ! -f "$snapshot/V2BX.md" || ! -f "$allowlist" ]]; then
  printf 'Missing source snapshot, marker, or patch allowlist\n' >&2
  exit 1
fi

mkdir "$tmpdir/module"
(
  cd "$tmpdir/module"
  go mod init source-snapshot-check >/dev/null
  go mod download -json "${module}@${version}" >"$tmpdir/module.json"
)

if ! grep -Fq "\"Hash\": \"$commit\"" "$tmpdir/module.json"; then
  printf 'Go module origin does not resolve to pinned commit %s\n' "$commit" >&2
  sed -n '1,80p' "$tmpdir/module.json" >&2
  exit 1
fi

source="$(go env GOMODCACHE)/${module}@${version}"
if [[ ! -d "$source" ]]; then
  printf 'Downloaded source directory is missing: %s\n' "$source" >&2
  exit 1
fi

if diff --strip-trailing-cr -qr "$source" "$snapshot" >"$tmpdir/tree.diff"; then
  diff_status=0
else
  diff_status=$?
fi
if [[ "$diff_status" -gt 1 ]]; then
  printf 'Unable to compare upstream and V2bX source trees\n' >&2
  cat "$tmpdir/tree.diff" >&2
  exit 1
fi

: >"$tmpdir/actual"
while IFS= read -r entry; do
  case "$entry" in
    "Files "*)
      remaining="${entry#Files "$source"/}"
      path="${remaining%% and "$snapshot"/*}"
      if [[ "$path" == "$remaining" ]]; then
        printf 'Unable to parse source diff entry: %s\n' "$entry" >&2
        exit 1
      fi
      ;;
    "Only in "*)
      remaining="${entry#Only in }"
      directory="${remaining%:*}"
      file="${remaining##*: }"
      case "$directory" in
        "$source") path="$file" ;;
        "$source"/*) path="${directory#"$source"/}/$file" ;;
        "$snapshot") path="$file" ;;
        "$snapshot"/*) path="${directory#"$snapshot"/}/$file" ;;
        *)
          printf 'Unable to parse source diff entry: %s\n' "$entry" >&2
          exit 1
          ;;
      esac
      ;;
    *)
      printf 'Unable to parse source diff entry: %s\n' "$entry" >&2
      exit 1
      ;;
  esac

  # Module archives omit submodules and nested Go modules.
  if [[ "$path" != "clients" && "$path" != clients/* ]]; then
    if [[ ! -f "$snapshot/test/go.mod" || ( "$path" != "test" && "$path" != test/* ) ]]; then
      printf '%s\n' "$path" >>"$tmpdir/actual"
    fi
  fi
done <"$tmpdir/tree.diff"

sed -E '/^[[:space:]]*#/d; /^[[:space:]]*$/d' "$allowlist" | LC_ALL=C sort -u >"$tmpdir/expected"
LC_ALL=C sort -u "$tmpdir/actual" -o "$tmpdir/actual"
if ! cmp -s "$tmpdir/expected" "$tmpdir/actual"; then
  printf 'Source differences do not match %s\n' "$allowlist" >&2
  diff -u "$tmpdir/expected" "$tmpdir/actual" >&2 || true
  exit 1
fi

printf '%s %s matches upstream commit %s with %s listed patch paths\n' \
  "$module" "$version" "$commit" "$(wc -l <"$tmpdir/actual" | tr -d ' ')"
