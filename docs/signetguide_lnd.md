# Signet Guide

This guide walks through the steps necessary to run the peerswap daemon on bitcoin signet and liquid testnet. This guide was written and tested under _Ubuntu-20.04_ but the same procedure also applies to different linux distributions.

## Install dependencies

Peerswap requires _lnd_, _bitcoind_ and an _elementsd_ installation if testing Liquid L-BTC swaps. If you already have all of these installed you can let them run in signet, or testnet mode and skip to the section about using the peerswap daemon.

## Bitcoind (signet)

Download the following files to install bitcoin-core.

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

Create config dir in home

```bash
mkdir -p ~/.bitcoin
```

Add signet config file

```bash
cat <<EOF > ~/.bitcoin/bitcoin.conf
signet=1
server=1
daemon=1
zmqpubrawblock=tcp://127.0.0.1:28332
zmqpubrawtx=tcp://127.0.0.1:28333
EOF
```

Start the bitcoin daemon in signet mode

```bash
bitcoind
```

## Liquid Testnet(optional)

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

The elements node now has to be synced on liquid testnet for the plugin to work. To check this, compare the _height_ value from

```shell
elements-cli getchaintips
```

with the height of the last block on [liquid-testnet-explorer](https://liquidtestnet.com/explorer)

## Lnd

_Note: If building from source, be sure to build with the required tags: `make install tags="signrpc walletrpc routerrpc invoicesrpc"` otherwise Peerswap will not work._

Download the following files
```bash
curl https://raw.githubusercontent.com/lightningnetwork/lnd/master/scripts/keys/roasbeef.asc | gpg --import \
wget https://github.com/lightningnetwork/lnd/releases/download/v0.15.2-beta/manifest-roasbeef-v0.15.2-beta.sig && \
wget https://github.com/lightningnetwork/lnd/releases/download/v0.15.2-beta/manifest-v0.15.2-beta.txt && \
wget https://github.com/lightningnetwork/lnd/releases/download/v0.15.2-beta/lnd-linux-amd64-v0.15.2-beta.tar.gz
```

Verify the release
```bash
gpg --verify manifest-roasbeef-v0.15.2-beta.sig manifest-v0.15.2-beta.txt
```

If the shasums match this command will return

`gpg: Good signature from "Olaoluwa Osuntokun <laolu32@gmail.com>" [ultimate]`

Extract the binaries 

```bash
tar -zvxf lnd-linux-amd64-v0.15.2-beta.tar.gz
```

Copy the binaries to the system path

```bash
sudo cp -vnR lnd-linux-amd64-v0.15.2-beta/* /usr/bin/
```

Create config dir in home

```bash
mkdir -p ~/.lnd
```

Add signet config file

```bash
cat <<EOF > ~/.lnd/lnd.conf
bitcoin.active=true
bitcoin.signet=true
bitcoin.node=bitcoind
listen=0.0.0.0:39735
EOF
```

Start Lnd in background

```bash
lnd </dev/null &>/dev/null &
```

Create a wallet with
```bash
lncli -n=signet create
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

Clone into the peerswap repository and build the peerswap plugin

```bash
git clone git@github.com:elementsproject/peerswap.git && \
cd peerswap && \
make lnd-release
```

Move the peerswap binaries to the systempath
```bash
sudo cp -vnR peerswapd /usr/bin/ && \
sudo cp -vnR pscli /usr/bin/
```

### Config file

In order to get peerswap running we need a configuration 

```bash
mkdir -p ~/.peerswap
```


Add signet config file. REPLACE USERNAME.

Bitcoin-swaps only config

```bash
cat <<EOF > ~/.peerswap/peerswap.conf
lnd.tlscertpath=/home/<username>/.lnd/tls.cert
lnd.macaroonpath=/home/<username>/.lnd/data/chain/bitcoin/signet/admin.macaroon
EOF
```

lbtc-swaps Config

```bash
cat <<EOF > ~/.peerswap/peerswap.conf
lnd.tlscertpath=/home/<username>/.lnd/tls.cert
lnd.macaroonpath=/home/<username>/.lnd/data/chain/bitcoin/signet/admin.macaroon
elementsd.rpcuser=admin1
elementsd.rpcpass=123 
elementsd.rpchost=http://127.0.0.1
elementsd.rpcport=18884
elementsd.rpcwallet=swaplnd
EOF
```

## Cleanup

Remove all unneccessary files and folders
```bash
rm go1.19.linux-amd64.tar.gz && \
rm lnd-linux-amd64-v0.15.2-beta.tar.gz && \
rm -r lnd-linux-amd64-v0.15.2-beta && \
rm manifest-roasbeef-v0.15.2-beta.sig && \
rm manifest-v0.15.2-beta.txt && \
rm SHA256SUMS && \
rm -r bitcoin-0.21.1/ && \
rm -r elements-elements-0.21.0.2/ && \
rm bitcoin-23.0-x86_64-linux-gnu.tar.gz && \
rm elements-elements-0.21.0.2-x86_64-linux-gnu.tar.gz && \
rm ELEMENTS-SHA256SUMS.asc && \
rm laanwj-releases.asc && \
rm SHA256SUMS.asc
```

### Run

start the peerswap daemon in background:

```bash
peerswapd </dev/null &>/dev/null &
```

Create a new signet address and receive some sats from https://signet.bc-2.jp/

```bash
lncli -n=signet newaddress p2wkh
```

Now connect to another node that has the peerswap plugin running, for example these development nodes run by @sputn1ck

cln node
```bash
lncli -n=signet connect 036ba9411c5bc0f07eaefa427d54973d8e06239c30caaef40775b3ac5c512cacf1@95.217.184.148:39375
```

lnd node
```bash
lncli -n=signet connect 0369aba787f74feb6c1ef1b7984569723b9eb88a1a7bc7323e67d796711d61a7d4@49.12.106.176:39735
```

Fund a channel to the connected peer, e.g. @sputn1ck node (replace the nodes pubkey and amount to your needs)

```bash
lncli -n=signet openchannel 02d5ee248489d76b54015df2938318a58ee0e35e4746579bd170efc7f1dd62e799 [amt] 
```

After the channel has been opened and is in `CHANNELD_NORMAL` state get the channel id per

```bash
lncli -n=signet listchannels | grep  "chan_id"
```


and try a swap-out

```bash
pscli swapout --sat_amt=[sat amount] --channel_id=[chan_id from above] --asset=btc
```

Note: The asset could also be `lbtc`. This will perform the swap on the bitcoin signet rather than the liquid testnet.

Get a new lbtc address and then generate some lbtc to the address via https://liquidtestnet.com/faucet

```bash
pscli lbtc-getaddress
```


