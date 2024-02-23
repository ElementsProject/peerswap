# Core Lightning Setup

This guide walks through the steps necessary to run the PeerSwap plugin for Bitcoin mainnet and Liquid swaps. This guide was written and tested under _Ubuntu-20.04_ but the same procedure also applies to different Linux distributions.

## Install dependencies

PeerSwap requires [Bitcoin Core](https://bitcoin.org/en/bitcoin-core/), [Core Lightning](https://github.com/ElementsProject/lightning) and an _elementsd_ installation for L-BTC swaps.

To setup `elementsd` for PeerSwap, refer to our [guide](https://github.com/ElementsProject/peerswap/blob/master/docs/setup_elementsd.md).

## PeerSwap

### Build

Install golang from https://golang.org/doc/install

Clone the PeerSwap repository and build the plugin

```bash
git clone https://github.com/ElementsProject/peerswap.git
cd peerswap
make cln-release
```

The `peerswap` binary is now located in the repo folder.



## Config file

In order to run `peerswap` add following lines to your the CLN config file:


```bash
plugin=/PATH/TO/peerswap
log-level=debug:plugin-peerswap
```

Specify the full path to the `peerswap` binary. For now it is recommended to log all debug messages from PeerSwap.

PeerSwap will automatically try to connect to your `bitcoind` (using the bitcoind RPC settings from CLN).

The default location of the swap database is `~/.lightning/bitcoin/peerswap/swaps`

Additional configuration can be specified in a `peerswap.conf` file that is expected to be located in the default PeerSwap data dir `~/.lightning/bitcoin/peerswap/` for mainnet.


The following optional configs can be specified:
```bash
# General section
policypath="/path/to/policy" ## Path to policy file (default: $HOME/.lightning/<network>/peerswap/policy.conf)

# Bitcoin section
# Alternative bitcoin rpc connection settings.
# Example config:
[Bitcoin]
rpcuser="user"
rpcpassword="password"
rpchost="http://host" ## the http:// is mandatory
rpcport=1234
cookiefilepath="/path/to/auth/.cookie" ## If set this will be used for authentication
bitcoinswaps=true ## If set to false, BTC mainchain swaps are disabled

# Liquid section
# Liquid rpc connection settings.
[Liquid]
rpcuser="user"
rpcpassword="password"
rpchost="http://host" ## the http:// is mandatory 
rpcport=1234
rpcpasswordfile="/path/to/auth/.cookie" ## If set this will be used for authentication
rpcwallet="swap-wallet" ## (default: peerswap)
liquidswaps=true ## If set to false, L-BTC swaps are disabled
```

In order to check if your daemon is setup correctly run

```bash
lightning-cli peerswap-reloadpolicy
```

### Policy

On first startup of the plugin a policy file will be generated (default path: `~/.lightning/bitcoin/peerswap/policy.conf`) in which trusted nodes will be specified.
This can be done manually by adding a line with `allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>` or with `lightning-cli peerswap-addpeer <PUBKEY>`.

>**Warning**  
>One could set the `accept_all_peers=true` policy to ignore the allowlist and allow all peers with direct channels to send swap requests.

### Debugging PeerSwap crashes

Currently if `peerswap` crashes, it will look like this in CLN's logs.

```
INFO    plugin-peerswap: Killing plugin: exited during normal operation
```

When this happens you can find the traceback in `~/.lightning/bitcoin/peerswap/peerswap-panic-log`. Look at the file timestamp to confirm it corresponds to the current crash. When you report an issue please include your CLN version, PeerSwap githash, this crash traceback, peerswap log messages during the event, and any other relevant details of what led to the failure.

We plan to improve this in [issue #6](https://github.com/ElementsProject/peerswap/issues/6) where glightning learns how to print the traceback via CLN's logging API.
