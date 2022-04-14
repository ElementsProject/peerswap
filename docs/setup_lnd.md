# LND Setup

This guide walks through the steps necessary to setup peerswap plugin with lnd.

## Install dependencies

Peerswap requires [LND](https://github.com/lightningnetwork/lnd) and if liquid should be used also a [liquid](https://docs.blockstream.com/liquid/node_setup.html) installation.

Install golang from https://golang.org/doc/install

## Peerswap

### Build

Clone the peerswap repository and build the peerswap plugin

```bash
git clone git@github.com:sputn1ck/peerswap.git && \
cd peerswap && \
make lnd-install
```

This will install `peerswapd` and `pscli` to your go path

### Config file

In order to get peerswap running we need a configuration 

```bash
mkdir -p ~/.peerswap
```

Add config file. Replace the paths to the tls certificate and macaroon file.

For every peer you want to allow swaps with add a line with `allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>`

Bitcoin-swaps only config

```bash
cat <<EOF > ~/.peerswap/peerswap.conf
lnd.tlscertpath=/home/<username>/.lnd/tls.cert
lnd.macaroonpath=/home/<username>/.lnd/data/chain/bitcoin/mainnet/admin.macaroon
network=mainnet
allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>
allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>
EOF
```

Liquid-swaps Config. Replace the rpc parameters as needed

```bash
cat <<EOF > ~/.peerswap/peerswap.conf
lnd.tlscertpath=/home/<username>/.lnd/tls.cert
lnd.macaroonpath=/home/<username>/.lnd/data/chain/bitcoin/mainnet/admin.macaroon
network=mainnet
bitcoinswaps=true
elementsd.rpcuser=<REPLACE_ME>
elementsd.rpcpass=<REPLACE_ME>
elementsd.rpchost=http://127.0.0.1
elementsd.rpcport=<REPLACE_ME>
elementsd.rpcwallet=swaplnd
accept_all_peers=true
allowlisted_peers=REPLACE_WITH_PUBKEY_OF_PEER
allowlisted_peers=REPLACE_WITH_PUBKEY_OF_PEER
EOF
```

__WARNING__: One could also set the `accept_all_peers=1` policy to ignore the allowlist and allow for all peers to send swap requests.

### Run

start the peerswap daemon in background:

```bash
peerswapd
```

In order to check if your daemon is setup correctly run
```bash
pscli reloadpolicy
```

