# Signet guide

clone peerswap
```bash
    git clone git@github.com:sputn1ck/peerswap.git
```
## Bitcoin Signet
```bash

    # download files
    wget https://bitcoin.org/bin/bitcoin-core-0.21.1/bitcoin-0.21.1-x86_64-linux-gnu.tar.gz
    wget https://bitcoin.org/bin/bitcoin-core-0.21.1/SHA256SUMS.asc
    wget https://bitcoin.org/laanwj-releases.asc

    # verify archive
    gpg --import laanwj-releases.asc
    gpg --verify SHA256SUMS.asc

    sha256sum -c SHA256SUMS.asc 2>&1 | grep OK

    # If all is OK, this will be the result:
    # bitcoin-0.21.1-x86_64-linux-gnu.tar.gz: OK

    # extract and copy binaries
    tar -zvxf bitcoin-0.21.1-x86_64-linux-gnu.tar.gz

    cp -vR bitcoin-0.21.1/* /usr/


    # start bitcoind
    bitcoind --signet --daemon
```
## Liquid Testnet
``` bash

# download files
wget https://github.com/ElementsProject/elements/releases/download/elements-0.18.1.12/elements-0.18.1.12-x86_64-linux-gnu.tar.gz
wget -O ELEMENTS-SHA256SUMS.asc https://github.com/ElementsProject/elements/releases/download/elements-0.18.1.12/SHA256SUMS.asc

# verify files
gpg --keyserver hkps://keyserver.ubuntu.com --recv-key "DE10E82629A8CAD55B700B972F2A88D7F8D68E87"
gpg --verify ELEMENTS-SHA256SUMS.asc

sha256sum -c ELEMENTS-SHA256SUMS.asc 2>&1 | grep OK

# If all is OK, this will be the result:
# elements-0.18.1.12-x86_64-linux-gnu.tar.gz: OK

# extract and install
tar -zvxf elements-0.18.1.12-x86_64-linux-gnu.tar.gz

cp -vR elements-0.18.1.12/* /usr/

# copy config file from peerswap to elements folder
cp peerswap/docs/elements.conf .elements/elements.conf

# start elements
elementsd --daemon

# NOTE: liquid-testnet must be synced in order for the plugin to work
```
## C-lightning
note: until c-lightning 0.11 we need to compile ourselves in order to get the necessary sendcustommsg command
``` bash
# NOTE: follow https://github.com/ElementsProject/lightning/blob/master/doc/INSTALL.md if not using ubuntu 15.04 or above

# get dependencies
sudo apt-get update
sudo apt-get install -y \
  autoconf automake build-essential git libtool libgmp-dev \
  libsqlite3-dev python3 python3-mako net-tools zlib1g-dev libsodium-dev \
  gettext

# clone and install clightning
git clone https://github.com/ElementsProject/lightning.git
cd lightning
./configure
make
sudo make install

```
## Peerswap
```
# install golang https://golang.org/doc/install

# clone peerswap repo if you haven't already

git clone git@github.com:sputn1ck/peerswap.git
cd peerswap
make release

# start lightningd NOTE: the peerswap plugin might not be located under root for you
lightningd --signet --daemon --log-file ~/l.log \
        --plugin=/root/peerswap/peerswap \
        --peerswap-liquid-rpchost=http://localhost \
        --peerswap-liquid-rpcport=18884 \
        --peerswap-liquid-rpcuser=admin1 \
        --peerswap-liquid-rpcpassword=123 \
        --peerswap-liquid-network=testnet \
        --peerswap-liquid-rpcwallet=swap


# goto https://signet.bc-2.jp/ and receive some signet coins
lightning-cli --signet newaddr

# connect to sputn1ck node
lightning-cli --signet connect 02a7d083fee7b4a47a93e9fddb1bc80500a3a9cf3976d21bcce393f79316e55072@49.12.106.176:39735

# fund a channel
lightning-cli --signet fundchannel 02a7d083fee7b4a47a93e9fddb1bc80500a3a9cf3976d21bcce393


# generate liquid address
lightning-cli --signet liquid-wallet-getaddress

# goto https://liquidtestnet.com/faucet and receive some testnet lbtc

# get channel short id
channel=$(lightning-cli --signet listfunds | jq '."channels"[0]."short_channel_id"')

# perform a swap out NOTE: you need liquid btc in order to pay for the swap
lightning-cli --signet swap-out 2000000 $channel
```

