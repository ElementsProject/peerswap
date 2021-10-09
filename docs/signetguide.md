# Signet Guide

This guide walks through the steps necessary to run the peerswap plugin on bitcoin signet and liquid testnet. This guide was written and tested under _Ubuntu-20.04_ but the same procedure also applies to different linux distributions.

## Install dependencies

Peerswap requires _clightning_, _bitcoind_ and if the liquid testnet should be used also an _elementsd_ installation. If you already have all of these installed you can let them run in signet, or testnet mode and skip to the section about using the plugin.

## Bitcoind (signet)

Download the following files to install bitcoin-core.

```bash
wget https://bitcoin.org/bin/bitcoin-core-0.21.1/bitcoin-0.21.1-x86_64-linux-gnu.tar.gz && \
wget https://bitcoin.org/bin/bitcoin-core-0.21.1/SHA256SUMS.asc && \
wget https://bitcoin.org/laanwj-releases.asc
```

Verify the downloaded data

```bash
gpg --import laanwj-releases.asc && \
gpg --verify SHA256SUMS.asc && \
sha256sum -c SHA256SUMS.asc 2>&1 | grep bitcoin-0.21.1-x86_64-linux-gnu.tar.gz
```

If the shasums match this command will return

`bitcoin-0.21.1-x86_64-linux-gnu.tar.gz: OK`

Extract the binaries

```bash
tar -zvxf bitcoin-0.21.1-x86_64-linux-gnu.tar.gz
```

Copy the binaries to the system path

```bash
sudo cp -vnR bitcoin-0.21.1/* /usr/
```

Start the bitoin daemon in signet mode

```bash
bitcoind --signet --daemon
```

## Elementsd (Liquid testnet)

Download the following files to install elementsd.

```bash
wget https://github.com/ElementsProject/elements/releases/download/elements-0.18.1.12/elements-0.18.1.12-x86_64-linux-gnu.tar.gz && \
wget -O ELEMENTS-SHA256SUMS.asc https://github.com/ElementsProject/elements/releases/download/elements-0.18.1.12/SHA256SUMS.asc
```

Verify the downloaded data

```bash
gpg --keyserver hkps://keyserver.ubuntu.com --recv-key "DE10E82629A8CAD55B700B972F2A88D7F8D68E87" && \
gpg --verify ELEMENTS-SHA256SUMS.asc && \
sha256sum -c ELEMENTS-SHA256SUMS.asc 2>&1 | grep OK
```

If the shasums match this command will return

`elements-0.18.1.12-x86_64-linux-gnu.tar.gz: OK`

Extract the binaries

```bash
tar -zvxf elements-0.18.1.12-x86_64-linux-gnu.tar.gz
```

Copy the binaries to the system path

```bash
sudo cp -vnR elements-0.18.1.12/* /usr/
```

Create config dir in home

```bash
mkdir -p ~/.elements
```

Add testnet config file (avoid to override existing config files)

```bash
cat <<EOF > ~/.elements/elements.conf
chain=liquidtestnetv1

server=1
listen=0
validatepegin=0
anyonecanspendaremine=1
peerbloomfilters=0
enforcenodebloom=1
txindex=1

# Liquid Testnet V1 (liquidtestnetv1) settings:
[liquidtestnetv1]
initialfreecoins=2100000000000000
con_dyna_deploy_start=999999999999
con_max_block_sig_size=150
addnode=liquid-testnet.blockstream.com:18891
signblockscript=51210209caad6d1e4fa3fddd4ee67f3e2aa9c280abfe3b30bfcd625874fe27e3e49e5e51ae
fallbackfee=0.00000100
rpcport=18884
rpcuser=admin1
rpcpassword=123
rpcallowip=0.0.0.0/0
rpcbind=0.0.0.0
addnode=49.12.106.176:18891
EOF
```

Start the daemon in testnet node

```shell
elementsd --daemon
```

### Wait for sync

The elements node now has to be synced on liquid testnet for the plugin to work. To check this, compare the _height_ value from

```shell
elements-cli getchaintips
```

with the height of the last block on [liquid-testnet-explorer](https://liquidtestnet.com/explorer)

## C-lightning

<!-- We need to build c-lightning ourselves to be able to use the required _sendcustommsg_ command -->

Download the necessary files.

```bash
wget https://github.com/ElementsProject/lightning/releases/download/v0.10.1/clightning-v0.10.1-Ubuntu-18.04.tar.xz && \
wget -O LIGHTNING-SHA256SUMS.asc https://github.com/ElementsProject/lightning/releases/download/v0.10.1/SHA256SUMS.asc && \
wget https://github.com/ElementsProject/lightning/releases/download/v0.10.1/SHA256SUMS
```

Verify the downloaded data

```bash
gpg --keyserver hkps://keyserver.ubuntu.com --recv-key "30DE693AE0DE9E37B3E7EB6BBFF0F67810C1EED1" && \
gpg --keyserver hkps://keyserver.ubuntu.com --recv-key "15EE8D6CAB0E7F0CF999BFCBD9200E6CD1ADB8F1" && \
gpg --keyserver hkps://keyserver.ubuntu.com --recv-key "B7C4BE81184FC203D52C35C51416D83DC4F0E86D" && \
gpg --verify LIGHTNING-SHA256SUMS.asc SHA256SUMS && \
sha256sum -c SHA256SUMS 2>&1 | grep clightning-v0.10.1-Ubuntu-18.04.tar.xz
```

If the shasums match this command will return

`clightning-v0.10.1-Ubuntu-18.04.tar.xz: OK`

Install dependencies

```bash
sudo apt-get install -y autoconf automake build-essential git libtool libgmp-dev libsqlite3-dev python3 python3-mako net-tools zlib1g-dev libsodium-dev gettext libpq5
```

Extract the binaries

```bash
tar -vxf clightning-v0.10.1-Ubuntu-18.04.tar.xz
```

Copy the binaries to the system path

```bash
sudo cp -vnR usr/* /usr/
```

Create config dir in home

```bash
mkdir -p ~/.lightning
```

Add signet config file

```bash
cat <<EOF > ~/.lightning/config
signet
bitcoin-datadir=$HOME/.bitcoin
addr=0.0.0.0:39375
log-level=debug
log-file=$HOME/.lightning/log
EOF
```

## Peerswap

### Build

Install golang from https://golang.org/doc/install

Clone into the peerswap repository and build the peerswap plugin

```bash
git clone git@github.com:sputn1ck/peerswap.git && \
cd peerswap && \
make release
```

### Policy

To ensure that only trusted nodes can send a peerswap request to your node it is necessary to create a policy in the lightning config dir (`~/lightning/policy.conf`) file in which the trusted nodes are specified. Change the following to your needs, replacing the _\<trusted node\>_ flag.

```bash
# ~/lightning/policy.conf
allowlisted_peers=<trusted node1>
allowlisted_peers=<trusted node2>
```

__WARNING__: One could also set the `accept_all_peers=1` policy to ignore the allowlist and allow for all peers to send swap requests.

### Run

start the c-lightning daemon with the following config flags

```bash
lightningd --daemon \
        --plugin=$HOME/peerswap/peerswap \
        --peerswap-liquid-rpchost=http://localhost \
        --peerswap-liquid-rpcport=18884 \
        --peerswap-liquid-rpcuser=admin1 \
        --peerswap-liquid-rpcpassword=123 \
        --peerswap-liquid-network=testnet \
        --peerswap-liquid-rpcwallet=swap \
        --peerswap-policy-path=$HOME/lightning/policy.conf
```

Create a new signet address and receive some sats from https://signet.bc-2.jp/

```bash
lightning-cli newaddr
```

Now connect to another node that has the peerswap plugin running, for example a development node run by @sputn1ck

```bash
lightning-cli connect 02a7d083fee7b4a47a93e9fddb1bc80500a3a9cf3976d21bcce393f79316e55072@49.12.106.176:39735
```

Fund a channel to the connected peer, e.g. @sputn1ck node (replace the nodes pubkey and amount to your needs)

```bash
lightning-cli fundchannel 02a7d083fee7b4a47a93e9fddb1bc80500a3a9cf3976d21bcce393f79316e55072 [amt] 
```

Get a new liquid address and then generate some lbtc to the address via https://liquidtestnet.com/faucet

```bash
lightning-cli peerswap-liquid-getaddress
```

After the channel has been funded and is in `CHANNELD_NORMAL` state get the short channel id per

```bash
lightning-cli listfunds | grep short_channel_id
```

and try a swap-out

```bash
lightning-cli peerswap-swap-out [amt] [short_channel_id] l-btc
```

Note: The asset could also be `btc`. This will perform the swap on the bitcoin signet rather than the liquid testnet.