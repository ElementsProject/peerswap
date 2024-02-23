# LND Setup

This guide walks through the steps necessary to setup the PeerSwap standalone daemon with LND for mainnet Bitcoin and Liquid swaps.

## Install dependencies

PeerSwap requires [LND](https://github.com/lightningnetwork/lnd). 

For L-BTC swaps, an `elementsd` installation is required. To setup `elementsd` for PeerSwap, check our [guide](https://github.com/ElementsProject/peerswap/blob/master/docs/setup_elementsd.md).

Install golang from https://golang.org/doc/install

## Peerswap

### Build

Clone the repository and build PeerSwap

```bash
git clone https://github.com/ElementsProject/peerswap.git && \
cd peerswap && \
make lnd-release
```

This will install `peerswapd` and `pscli` to your GOPATH

### Config file

In order to get PeerSwap running we need a configuration 

```bash
mkdir -p ~/.peerswap
```

Add config file. Replace the paths to the TLS certificate and macaroon file.

For every peer you want to allow swaps with add a line with `allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>`

BTC swaps only config:

```bash
cat <<EOF > ~/.peerswap/peerswap.conf
lnd.tlscertpath=/home/<username>/.lnd/tls.cert
lnd.macaroonpath=/home/<username>/.lnd/data/chain/bitcoin/mainnet/admin.macaroon
EOF
```

BTC and L-BTC swaps config. Replace the RPC parameters as needed:

```bash
cat <<EOF > ~/.peerswap/peerswap.conf
lnd.tlscertpath=/home/<username>/.lnd/tls.cert
lnd.macaroonpath=/home/<username>/.lnd/data/chain/bitcoin/mainnet/admin.macaroon
elementsd.rpcuser=<REPLACE_ME>
elementsd.rpcpass=<REPLACE_ME>
elementsd.rpchost=http://127.0.0.1 # the http:// is mandatory
elementsd.rpcport=<REPLACE_ME>
elementsd.rpcwallet=peerswap
elementsd.liquidswaps=true # set to false to manually disable L-BTC swaps
EOF
```

L-BTC only config. 

```bash
cat <<EOF > ~/.peerswap/peerswap.conf

bitcoinswaps=false # disables BTC swaps
lnd.tlscertpath=/home/<username>/.lnd/tls.cert
lnd.macaroonpath=/home/<username>/.lnd/data/chain/bitcoin/mainnet/admin.macaroon
elementsd.rpcuser=<REPLACE_ME>
elementsd.rpcpass=<REPLACE_ME>
elementsd.rpchost=http://127.0.0.1 # the http:// is mandatory
elementsd.rpcport=<REPLACE_ME>
elementsd.rpcwallet=peerswap
EOF
```

### Policy

On first startup of the plugin a policy file will be generated (default path: `~/.peerswap/policy.conf`) in which trusted nodes will be specified.
This can be done manually by adding a line with `allowlisted_peers=<REPLACE_WITH_PUBKEY_OF_PEER>` or with `pscli addpeer <PUBKEY>`.

>**Warning**  
>One could set the `accept_all_peers=true` policy to ignore the allowlist and allow all peers with direct channels to send swap requests.

### Run

Start the PeerSwap daemon in background:

```bash
peerswapd
```

In order to check if your daemon is setup correctly run
```bash
pscli reloadpolicy
```

