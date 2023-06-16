# Signet Guide

This guide walks through the steps necessary to run the PeerSwap plugin on Bitcoin signet and Liquid testnet. This guide was written and tested under _Ubuntu-20.04_ but the same procedure also applies to different Linux distributions.

## Install dependencies

PeerSwap requires _core-lightning_, _bitcoind_ and an _elementsd_ installation if testing Liquid L-BTC swaps. If you already have all of these installed you can let them run in signet, or testnet mode and skip to the section about using the plugin.

## Bitcoind (signet)

Download the following files to install Bitcoin Core.

```bash
wget https://bitcoincore.org/bin/bitcoin-core-23.0/bitcoin-23.0-x86_64-linux-gnu.tar.gz && \
wget https://bitcoincore.org/bin/bitcoin-core-23.0/SHA256SUMS.asc && \
wget https://bitcoin.org/laanwj-releases.asc
```

Verify the downloaded data

```bash
gpg --import laanwj-releases.asc && \
gpg --verify SHA256SUMS.asc && \
grep bitcoin-23.0-x86_64-linux-gnu.tar.gz && \
sha256sum -c SHA256SUMS.asc 2>&1 
```

If the shasums match this command will return

`bitcoin-0.23.0-x86_64-linux-gnu.tar.gz: OK`

Extract the binaries

```bash
tar -zvxf bitcoin-23.0-x86_64-linux-gnu.tar.gz
```

Copy the binaries to the system path

```bash
sudo cp -vnR bitcoin-23.0/* /usr/
```

Start the Bitcoin daemon in signet mode

```bash
bitcoind --signet --daemon
```

## Liquid testnet (optional)

Download the following files to install elementsd.

```bash
wget https://github.com/ElementsProject/elements/releases/download/elements-0.21.0.2/elements-elements-0.21.0.2-x86_64-linux-gnu.tar.gz && \
wget -O ELEMENTS-SHA256SUMS.asc https://github.com/ElementsProject/elements/releases/download/elements-0.21.0.2/SHA256SUMS.asc
```

Verify the downloaded data

```bash
gpg --keyserver hkps://keyserver.ubuntu.com --recv-key "DE10E82629A8CAD55B700B972F2A88D7F8D68E87" && \
gpg --verify ELEMENTS-SHA256SUMS.asc && \
sha256sum -c ELEMENTS-SHA256SUMS.asc 2>&1 | grep OK
```

If the shasums match this command will return

`elements-elements-0.21.0.2-x86_64-linux-gnu.tar.gz: OK`

Extract the binaries

```bash
tar -zvxf elements-elements-0.21.0.2-x86_64-linux-gnu.tar.gz
```

Copy the binaries to the system path

```bash
sudo cp -vnR elements-elements-0.21.0.2/* /usr/
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
evbparams=dynafed:0:::
multi_data_permitted=1
EOF
```

Start the daemon in testnet node

```shell
elementsd --daemon
```

### Wait for sync

The elements node now has to be synced on Liquid testnet for the plugin to work. To check this, compare the _height_ value from

```shell
elements-cli getchaintips
```

with the height of the last block on [Liquid testnet explorer](https://blockstream.info/liquidtestnet/)

## Core Lightning

<!-- We need to build CLN ourselves to be able to be interoperable with LND on signet -->

Follow the build instructions [here](https://github.com/ElementsProject/lightning/blob/master/doc/INSTALL.md#to-build-on-ubuntu).

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
wget https://go.dev/dl/go1.19.linux-amd64.tar.gz && \
sudo rm -rf /usr/local/go && \
sudo tar -C /usr/local -xzf go1.19.linux-amd64.tar.gz && \
export PATH=$PATH:/usr/local/go/bin
```

Clone into the PeerSwap repository and build the plugin

```bash
git clone git@github.com:elementsproject/peerswap.git && \
cd peerswap && \
make cln-release
```

### Cleanup

Remove all unnecessary files and folders
```bash
rm go1.17.3.linux-amd64.tar.gz && \
rm SHA256SUMS && \
rm -r bitcoin-0.21.1 && \
rm -r elements-elements-0.21.0/ && \
rm bitcoin-0.21.1-x86_64-linux-gnu.tar.gz && \
rm elements-elements-0.21.0.2-x86_64-linux-gnu.tar.gz && \
rm ELEMENTS-SHA256SUMS.asc && \
rm laanwj-releases.asc && \
rm SHA256SUMS.asc
```

### Config (Enable Liquid)

PeerSwap will try to detect the `elementsd` cookie file at the default location.
If you use a different data fir for `elementsd` you need to add the connection
options to the PeerSwap config file in order to enable Liquid swaps.
```
mkdir -p $HOME/.lightning/signet/peerswap
touch $HOME/.lightning/signet/peerswap/peerswap.conf
```

```
echo '[Liquid]
rpcuser="admin1"
rpcpassword="123"
rpchost="http://localhost"
rpcport=18884
rpcwallet="swap"' > $HOME/.lightning/signet/peerswap.conf
```

To disable Liquid swaps you can add the following to the PeerSwap config file:
```
[Liquid]
disabled=true
```

### Run

Start the CLN daemon with:

```bash
lightningd --daemon \
        --plugin=$HOME/peerswap/peerswap 
``` 

Create a new signet address and receive some sats from https://signet.bc-2.jp/

```bash
lightning-cli newaddr
```

Now connect to another node that has the PeerSwap plugin running, for example these development nodes run by @sputn1ck

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
lightning-cli fundchannel 0369aba787f74feb6c1ef1b7984569723b9eb88a1a7bc7323e67d796711d61a7d4 [amt] 
```

Get a new testnet L-BTC address and then generate some L-BTC to the address via https://liquidtestnet.com/faucet

```bash
lightning-cli peerswap-lbtc-getaddress
```

Add the peer to the allowlist
```bash
lightning-cli peerswap-addpeer 0369aba787f74feb6c1ef1b7984569723b9eb88a1a7bc7323e67d796711d61a7d4
```

After the channel has been funded and is in `CHANNELD_NORMAL` state get the short channel id per

```bash
lightning-cli listfunds | grep short_channel_id
```

and try a swap-out

```bash
lightning-cli peerswap-swap-out [amt] [short_channel_id] lbtc
```

Note: The asset could also be `btc`. This will perform the swap on the Bitcoin signet rather than the Liquid testnet.

