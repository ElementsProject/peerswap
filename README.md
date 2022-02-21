# 🔴 DO NOT USE ON MAINNET YET🔴 

## PeerSwap

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes.

It allows rebalancing of your channels using on-chain assets.

See [Spec](./docs/spec.md) for details

Features:

Assets:

- [x] Rebalancing of channels using l-btc
- [x] Rebalancing of channels using btc

Nodes:
- [x] c-lightning
- [X] lnd

### Usage


#### clightning
For a bitcoin-signetnet / liquid-testnet setup guide see this [guide](./docs/signetguide_clightning.md)

Details on general usage can be found [here](./docs/usage.md)


#### lnd
For a bitcoin-signetnet / liquid-testnet setup guide see this [guide](./docs/signetguide_lnd.md)

Details on general usage to follow...

### Upgrading

In order to upgrade PeerSwap, no swaps should be unfinished.

To check for active swaps run:

 - lnd: `pscli listactiveswaps`
 - c-lightning: `lightning-cli peerswap-listactiveswaps`

If no swaps are returned, you can safely upgrade peerswap

#### Reject new requests

If you are an active node with frequent incoming swap request you can run the following conmand to stop accepting swap requests.

 - lnd: `pscli rejectswaps true`
 - c-lightning: `lightning-cli peerswap-rejectswaps true`

To revert run: 

 - lnd: `pscli rejectswaps false`
 - c-lightning: `lightning-cli peerswap-rejectswaps false`


#### Upgrade failures

If you have active swaps running and try to upgrade, peerswap will not start up. You should see an error message in your logs.
You need to downgrade peerswap to the previous version in order to complete the swaps.
### Development

PeerSwap uses the [nix](https://nixos.org/download.html) package manager for a simple development environment
In order to start hacking, install nix and run `nix-shell`. This will fetch all dependencies (bar golang).
