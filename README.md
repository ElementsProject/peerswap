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

### Development

PeerSwap uses the [nix](https://nixos.org/download.html) package manager for a simple development environment
In order to start hacking, install nix and run `nix-shell`. This will fetch all dependencies (bar golang).
