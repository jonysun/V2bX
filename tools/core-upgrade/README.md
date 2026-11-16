# Core Upgrade Checker

`check.sh` is a read-only consistency gate for a prepared kernel candidate and a release gate for the current checkout.

Run static checks without Docker from PowerShell:

```powershell
& "C:\Program Files\Git\bin\bash.exe" -lc "tools/core-upgrade/check.sh --mode pre-update --skip-build"
```

Verify that the pinned upstream tags still resolve to the recorded commits:

```powershell
& "C:\Program Files\Git\bin\bash.exe" -lc "tools/core-upgrade/check.sh --mode pre-update --skip-build --verify-upstream"
```

Run a single-core or full release build in the Go 1.27 Docker image:

```powershell
& "C:\Program Files\Git\bin\bash.exe" -lc "tools/core-upgrade/check.sh --mode release --core hysteria2"
& "C:\Program Files\Git\bin\bash.exe" -lc "tools/core-upgrade/check.sh --mode release --core sing"
& "C:\Program Files\Git\bin\bash.exe" -lc "tools/core-upgrade/check.sh --mode release --core xray"
& "C:\Program Files\Git\bin\bash.exe" -lc "tools/core-upgrade/check.sh --mode release --core all"
```

The checker never edits `go.mod`, `go.sum`, source files, or release metadata. It writes a Markdown report under `tools/core-upgrade/reports/` and exits non-zero when a required check fails. `pre-update` checks that the manifest, module pins, source markers, and toolchain agree in the checkout being evaluated. It does not predict compatibility for an arbitrary version that has not yet been integrated, and it does not generate or update the V2bX forks. Prepare a candidate snapshot and dependency pins first, then use the release gate to prove build and package behavior. `conf.TestConf_Watch` is intentionally excluded from runtime tests because it blocks forever; `cmd.TestRun` is compile-checked only because it starts the application.

The manifest in `versions.env` pins upstream tags, source commits, Go module versions, and the shared QUIC dependency. `third_party/*/V2BX.md` explains each local source patch boundary, while `*-patch-files.txt` lists every expected file difference from the official source archive. Release mode downloads the pinned Go modules, confirms their origin commits, and rejects any source-tree drift outside those allowlists. Upstream submodules and nested Go modules omitted from module archives are excluded from that comparison. `--verify-upstream` checks live tag refs when network access is available; a connection failure is reported separately from a missing tag. A target version is a candidate until its compatibility report and full release build pass.

`third_party/qpack-compat` is a narrow compatibility bridge based on qpack v0.6. It keeps the old `NewDecoder(callback)` and `DecodeFull` API required by the current sing-box/Xray QUIC forks while allowing Hysteria's newer qpack API. Remove this bridge after both QUIC forks have moved to the current qpack API.
