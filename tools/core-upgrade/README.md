# Core Upgrade Checker

`check.sh` is the read-only gate to run before changing kernel dependencies and before producing a release candidate.

Run static checks without Docker:

```bash
bash tools/core-upgrade/check.sh --mode pre-update --skip-build
```

Run a single-core or full release build in the Go 1.27 Docker image:

```bash
bash tools/core-upgrade/check.sh --mode release --core hysteria2
bash tools/core-upgrade/check.sh --mode release --core sing
bash tools/core-upgrade/check.sh --mode release --core xray
bash tools/core-upgrade/check.sh --mode release --core all
```

The checker never edits `go.mod`, `go.sum`, source files, or release metadata. It writes a Markdown report under `tools/core-upgrade/reports/` and exits non-zero when a required check fails.

The manifest in `versions.env` records the target release versions. A target version is a candidate until its compatibility report and full release build pass.
