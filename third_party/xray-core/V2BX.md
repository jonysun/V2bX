# V2bX Xray fork

This module is based on the official Xray-core `v26.11.9` release at commit
`676eaf66f8f05d4d558ee7a74b4736623d3feeb9`. Go resolves that release to module
version `v1.260327.1-0.20261109052709-676eaf66f8f0` because the module path does
not include `/v26`.

V2bX compatibility patches adapt the MASQUE HTTP/3 stream API to the newer
shared `github.com/apernet/quic-go` version required by Hysteria 2.14.0,
restore Shadowsocks IV replay checks from the previous V2bX fork, and keep
the Shadowsocks 2022 inbound registered in the `all` distro.

The old `none`/`plain` Shadowsocks method is intentionally rejected by V2bX:
it has no authentication and cannot safely support XBoard's dynamic
multi-user model.
