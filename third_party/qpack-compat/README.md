# qpack compatibility bridge

This package is based on `github.com/quic-go/qpack` v0.6.0 and adds two source-compatible entry points used by older QUIC forks:

- `NewDecoder(callback)` alongside the v0.6 zero-argument form;
- `Decoder.DecodeFull` alongside the v0.6 iterator API.

The bridge exists because Hysteria v2.14 uses qpack v0.6 while the current sing-box and Xray QUIC forks still call the v0.5 API. Remove the root module replacement after those forks migrate to the current API.
