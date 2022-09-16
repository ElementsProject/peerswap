# core-lightning Setup

This guide walks through the steps necessary to run the peerswap plugin on bitcoin signet and liquid testnet. This guide was written and tested under _Ubuntu-20.04_ but the same procedure also applies to different linux distributions.

## Install dependencies

Peerswap requires [Bitcoin Core](https://bitcoin.org/en/bitcoin-core/), [core-lightning](https://github.com/ElementsProject/lightning) and if the liquid testnet should be used also an _elementsd_ installation. If you already have all of these installed you can let them run in signet, or testnet mode and skip to the section about using the plugin.

## Peerswap

### Build

Install golang from https://golang.org/doc/install

Clone into the peerswap repository and build the peerswap plugin

```bash
git clone https://github.com/ElementsProject/peerswap.git
cd peerswap
make cln-release
```

The `peerswap-plugin` binary is now located in the repo folder.



## Config file

In order to run `peerswap` add following lines to your the core-lightning config file:


```bash
plugin=~./lightning/plugins/peerswap-plugin
log-level=debug:plugin-peerswap-plugin
```

Specify the full path to the `peerswap-plugin` binary. For now it is recommended to log all debug messages from peerswap.

Peerswap will automatically try to connect to your bitcoind and (if available) elementsd

The following optional configs can be specified:

```bash
# General
peerswap-db-path ## Path to swap db file (default: $HOME/.lightning/<network>/peerswap/swap)
peerswap-policy-path ## Path to policy file (default: $HOME/.lightning/<network>/peerswap/policy.conf)

# Bitcoin connection info 
peerswap-bitcoin-rpchost ## Host of bitcoind rpc (default: localhost)
peerswap-bitcoin-rpcport ## Port of bitcoind rpc (default: network-default)
peerswap-bitcoin-rpcuser ## User for bitcoind rpc
peerswap-bitcoin-rpcpassword ## Password for bitcoind rpc
peerswap-bitcoin-cookiefilepath ## Path to bitcoin cookie file 

peerswap-elementsd-enabled ## Override liquid enable (default: true)
peerswap-elementsd-rpchost ## Host of elementsd rpc (default: localhost)
peerswap-elementsd-rpcport ## Port of elementsd rpc (default: 18888)
peerswap-elementsd-rpcuser ## User for elementsd rpc
peerswap-elementsd-rpcpassword ## Password for elementsd rpc
peerswap-elementsd-rpcpasswordfile ## Path to passwordfile for elementsd rpc
peerswap-elementsd-rpcwallet ## Rpcwallet to use (default: peerswap)
```

In order to check if your daemon is setup correctly run

```bash
lightning-cli peerswap-reloadpolicy
```

### Policy

On first startup of the plugin a policy file will be generated (default path: `~/.lightning/<network>/peerswap/policy.conf`) in which trusted nodes will be specified.
This can be done manually by adding a line with `allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>` or with `lightning-cli peerswap-addpeer <PUBKEY>`.

__WARNING__: One could set the `accept_all_peers=true` policy to ignore the allowlist and allow all peers with direct channels to send swap requests.

### Debugging peerswap-plugin crashes

Currently if `peerswap-plugin` crashes looks like this in lightningd's log.

```
INFO    plugin-peerswap-plugin: Killing plugin: exited during normal operation
```

When this happens you can find the traceback in `~/.lightning/bitcoin/peerswap/peerswap-panic-log`. Look at the file timestap to confirm it corresponds to the current crash. When you report an issue please include your CLN version, PeerSwap githash, this crash traceback, peerswap log messages during the event, and any other relevant details of what led to the failure.

We plan to improve this in [issue #6](https://github.com/ElementsProject/peerswap/issues/6) where glightning learns how to print the traceback via CLN's logging API.
