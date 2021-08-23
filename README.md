# 🔴 DO NOT USE ON MAINNET YET🔴 

## PeerSwap

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes.

It allows rebalancing of your channels using on-chain assets.

See [Spec](./docs/spec.md) for details

Features:

Assets:

- [x] Rebalancing of channels using l-btc
- [ ] Rebalancing of channels using btc

Nodes:
- [x] c-lightning
- [ ] lnd requires [lightningnetwork/lnd#5346](https://github.com/lightningnetwork/lnd/pull/5346)

### Usage

For a bitcoin-signetnet / liquid-testnet setup guide see this [guide](./docs/signetguide.md)

In order to use peerswap start `lightningd` with the following options, replacing as necessary
```
lightningd \ 
 --peerswap-liquid-rpchost=http://localhost \
 --peerswap-liquid-rpcport=7041 \
 --peerswap-liquid-rpcuser=admin1 \
 --peerswap-liquid-rpcpassword=123 \
 --peerswap-liquid-network=regtest \
 --peerswap-liquid-rpcwallet=swap1
```

Details on usage can be found [here](./docs/usage.md)

### Development

PeerSwap uses the [nix](https://nixos.org/download.html) package manager for a simple development environment
In order to start hacking, install nix and run `nix-shell`. This will fetch all dependencies (bar golang).
