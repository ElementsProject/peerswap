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
- [ ] lnd requires [lightningnetwork/lnd#5346](https://github.com/lightningnetwork/lnd/pull/5346)

### Usage

For a bitcoin-signetnet / liquid-testnet setup guide see this [guide](./docs/signetguide.md)

Details on general usage can be found [here](./docs/usage.md)

### Development

PeerSwap uses the [nix](https://nixos.org/download.html) package manager for a simple development environment
In order to start hacking, install nix and run `nix-shell`. This will fetch all dependencies (bar golang).
