# V2bX sing-box fork

This module is based on the official sing-box `v1.14.4` release at commit
`c005f35300ce76427dc285481ec5695f5b179cd2`.

V2bX carries a small compatibility patch set for its server integration:

- expose router context and dynamic inbound removal;
- allow a started inbound manager to create and remove listeners safely;
- preserve Reality `xver` from panel node settings;
- expose protocol user add/delete methods used by XBoard/V2Board sync.

Keep the upstream tag and commit fixed in upgrade reports. Review every patch
against the next upstream tag before updating this fork.
