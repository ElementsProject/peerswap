# 🔴 DO NOT USE ON MAINNET YET🔴 

## PeerSwap

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes.

It allows rebalancing of your channels using on-chain assets.

Features:

Assets:

- [x] Rebalancing of channels using l-btc
- [ ] Rebalancing of channels using btc

Nodes:
- [x] c-lightning
- [ ] lnd requires [lightningnetwork/lnd#5346](https://github.com/lightningnetwork/lnd/pull/5346)

### Usage

In order to use peerswap start `lightningd` with the following options, replacing as necessary
```
lightningd \ 
 --peerswap-liquid-rpchost=http://localhost \
 --peerswap-liquid-rpcport=7041 \
 --peerswap-liquid-rpcuser=admin1 \
 --peerswap-liquid-rpcpassword=123 \
 --peerswap-liquid-network=regtest
```

### Development

PeerSwap uses [nigiri](https://github.com/vulpemventures/nigiri) 
for local testing, as well as a hacky nix-shell script 
for setting up two clightning nodes
