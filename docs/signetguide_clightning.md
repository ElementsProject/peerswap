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

## Liquid Testnet(optional)

Download the following files to install elementsd.

```bash
wget https://github.com/ElementsProject/elements/releases/download/elements-0.21.0/elements-elements-0.21.0-x86_64-linux-gnu.tar.gz && \
wget -O ELEMENTS-SHA256SUMS.asc https://github.com/ElementsProject/elements/releases/download/elements-0.21.0/SHA256SUMS.asc
```

Verify the downloaded data

```bash
gpg --keyserver hkps://keyserver.ubuntu.com --recv-key "DE10E82629A8CAD55B700B972F2A88D7F8D68E87" && \
gpg --verify ELEMENTS-SHA256SUMS.asc && \
sha256sum -c ELEMENTS-SHA256SUMS.asc 2>&1 | grep OK
```

If the shasums match this command will return

`elements-elements-0.21.0-x86_64-linux-gnu.tar.gz: OK`

Extract the binaries

```bash
tar -zvxf elements-elements-0.21.0-x86_64-linux-gnu.tar.gz
```

Copy the binaries to the system path

```bash
sudo cp -vnR elements-elements-0.21.0/* /usr/
```

Create config dir in home

```bash
mkdir -p ~/.elements
```

Add testnet config file (avoid to override existing config files)

```bash
cat <<EOF > ~/.elements/elements.conf
chain=liquidtestnet
# Liquid Testnet (liquidtestnet) settings:
[liquidtestnet]

# General settings:
listen=1
txindex=1
validatepegin=0
anyonecanspendaremine=0
initialfreecoins=2100000000000000
con_dyna_deploy_start=0
con_max_block_sig_size=150
checkblockindex=0 
addnode=liquid-testnet.blockstream.com:18892
addnode=liquidtestnet.com:18891
fallbackfee=0.00000100
daemon=1
con_has_parent_chain=0
parentgenesisblockhash=NULL
pubkeyprefix=36
scriptprefix=19
blindedprefix=23
bech32_hrp=tex
blech32_hrp=tlq
pchmessagestart=410edd62
dynamic_epoch_length=1000
signblockscript=51210217e403ddb181872c32a0cd468c710040b2f53d8cac69f18dad07985ee37e9a7151ae

rpcport=18884
rpcuser=admin1
rpcpassword=123
rpcbind=127.0.0.1
addnode=95.217.184.148:18444
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

<!-- We need to build c-lightning ourselves to be able to be interoperable with lnd on signet -->

get dependencies

```bash
sudo apt-get update && \
sudo apt-get install -y \
autoconf automake build-essential git libtool libgmp-dev \
libsqlite3-dev python3 python3-mako net-tools zlib1g-dev libsodium-dev jq \
gettext
```

clone and install clightning

```bash
git clone https://github.com/sputn1ck/lightning.git && \
cd lightning && \
git checkout origin/v0.10.2 && \
./configure
```
```
sudo make && \
sudo make install
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
```bash
wget https://go.dev/dl/go1.17.3.linux-amd64.tar.gz && \
sudo rm -rf /usr/local/go && \
sudo tar -C /usr/local -xzf go1.17.3.linux-amd64.tar.gz && \
export PATH=$PATH:/usr/local/go/bin
```

Clone into the peerswap repository and build the peerswap plugin

```bash
git clone git@github.com:sputn1ck/peerswap.git && \
cd peerswap && \
make cln-release
```

### Policy

To ensure that only trusted nodes can send a peerswap request to your node it is necessary to create a policy in the lightning config dir (`~/lightning/policy.conf`) file in which the trusted nodes are specified. Change the following to your needs, replacing the _\<trusted node\>_ flag.
For Signet testing we add accept_all_peers=1
```bash
cat <<EOF > ~/.lightning/policy.conf
accept_all_peers=1
EOF
```

## Cleanup

Remove all unneccessary files and folders
```bash
rm go1.17.3.linux-amd64.tar.gz && \
rm clightning-v0.10.2-Ubuntu-20.04.tar.xz && \
rm -r usr/ && \
rm LIGHTNING-SHA256SUMS.asc && \
rm SHA256SUMS && \
rm -r bitcoin-0.21.1/ && \
rm -r elements-elements-0.21.0/ && \
rm bitcoin-0.21.1-x86_64-linux-gnu.tar.gz && \
rm elements-elements-0.21.0-x86_64-linux-gnu.tar.gz && \
rm ELEMENTS-SHA256SUMS.asc && \
rm laanwj-releases.asc && \
rm SHA256SUMS.asc
```

### Run

start the c-lightning daemon with the following config flags for bitcoin only:

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
        --peerswap-liquid-rpcuser=admin1 \
        --peerswap-liquid-rpcpassword=123 \
        --peerswap-liquid-network=testnet \
        --peerswap-liquid-rpcwallet=swap \
        --peerswap-policy-path=$HOME/.lightning/policy.conf
```

Create a new signet address and receive some sats from https://signet.bc-2.jp/

```bash
lightning-cli newaddr
```

Now connect to another node that has the peerswap plugin running, for example these development nodes run by @sputn1ck

cln node
```bash
lightning-cli -n=signet connect 0 36ba9411c5bc0f07eaefa427d54973d8e06239c30caaef40775b3ac5c512cacf1@95.217.184.148:39375
```

lnd node
```bash
lightning-cli connect 0369aba787f74feb6c1ef1b7984569723b9eb88a1a7bc7323e67d796711d61a7d4@49.12.106.176:39735
```

Fund a channel to the connected peer, e.g. @sputn1ck node (replace the nodes pubkey and amount to your needs)

```bash
lightning-cli fundchannel 02d5ee248489d76b54015df2938318a58ee0e35e4746579bd170efc7f1dd62e799 [amt] 
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

