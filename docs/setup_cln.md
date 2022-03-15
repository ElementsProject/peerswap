# c-lightning Setup

This guide walks through the steps necessary to run the peerswap plugin on bitcoin signet and liquid testnet. This guide was written and tested under _Ubuntu-20.04_ but the same procedure also applies to different linux distributions.

## Install dependencies

Peerswap requires [Bitcoin Core](https://bitcoin.org/en/bitcoin-core/), [c-lightning](https://github.com/ElementsProject/lightning) and if the liquid testnet should be used also an _elementsd_ installation. If you already have all of these installed you can let them run in signet, or testnet mode and skip to the section about using the plugin.

## Peerswap

### Build

Install golang from https://golang.org/doc/install

Clone into the peerswap repository and build the peerswap plugin

```bash
git clone git@github.com:sputn1ck/peerswap.git && \
cd peerswap && \
make cln-release
```

The `peerswap` binary is now located in the repo folder.

### Policy

To ensure that only trusted nodes can send a peerswap request to your node it is necessary to create a policy in the lightning config dir (e.g. `~/.lightning/policy.conf`) file in which the trusted nodes are specified. For every peer you want to allow swaps with, add a line with `allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>`

```bash
cat <<EOF > ~/.lightning/policy.conf
allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>
allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>
EOF
```

## Run

In order to run `peerswap` start the c-lightning daemon with the following config flags replacing as needed.

For bitcoin only:

```bash
lightningd --daemon \
        --plugin=$HOME/peerswap/peerswap \
        --peerswap-policy-path=$HOME/.lightning/policy.conf
```
Or with liquid enabled

```bash
lightningd --daemon \
        --plugin=$HOME/peerswap/peerswap \
        --peerswap-liquid-rpchost=http://localhost \
        --peerswap-liquid-rpcport=18884 \
        --peerswap-liquid-rpcuser=<REPLACE_ME> \
        --peerswap-liquid-rpcpassword=<REPLACE_ME> \
        --peerswap-liquid-rpcwallet=swap \
        --peerswap-policy-path=$HOME/.lightning/policy.conf
```

__WARNING__: One could also set the `accept_all_peers=1` policy to ignore the allowlist and allow for all peers to send swap requests.


In order to check if your daemon is setup correctly run
```bash
lightning-cli peerswap-reloadpolicy
```
