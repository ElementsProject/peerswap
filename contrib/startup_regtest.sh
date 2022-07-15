#!/bin/bash

start_docker_env() {
  docker-compose -f .ci/docker/docker-compose.yml up -d --remove-orphans
}

stop_docker_env() {
  docker-compose -f .ci/docker/docker-compose.yml down
}

stop_nodes() {
  if [ -z "$2" ]; then
    network=regtest
  else
    network="$2"
  fi
  if [ -n "$LN_NODES" ]; then
    for i in $(seq $LN_NODES); do
      test ! -f "/tmp/l$i-$network/lightningd-$network.pid" ||
        (
          node_pid=$(cat "/tmp/l$i-$network/lightningd-$network.pid")
          echo "stopping node $i with pid $node_pid"
          kill $node_pid
          rm "/tmp/l$i-$network/lightningd-$network.pid"
          unset node_pid
        )
      echo "$(lncli-1 stop)"
      echo "$(lncli-2 stop)"
      unalias "l$i-cli"
      unalias "l$i-log"
    done
  fi
}

remove_nodes() {
  stop_nodes
  if [ -z "$1" ]; then
    network=regtest
  else
    network="$1"
  fi
  if [ -n "$LN_NODES" ]; then
    for i in $(seq $LN_NODES); do
      LN_DIR="/tmp/l$i-$network"
      rm -rf "/tmp/lnd-regtest-$i"
      if [ -d $LN_DIR ]; then
        echo "removing node from $LN_DIR"
        rm -rf $LN_DIR
      fi
    done
  fi
  unset LN_DIR
}

start_nodes_lnd() {
  LND='lnd'
  if [ -z "$1" ]; then
    node_count=2
  else
    node_count=$1
  fi
  LND_NODES=$node_count
  for i in $(seq $node_count); do
    rpcport=$((10101 + i * 100))
    listenport=$((10102 + i * 100))
    mkdir -p "/tmp/lnd-regtest-$i/data"
    touch "/tmp/lnd-regtest-$i/data/lnd.conf"
    # Start the lightning nodes
    lnd --datadir=/tmp/lnd-regtest-$i/data \
    --bitcoin.active --bitcoin.regtest --bitcoin.node=bitcoind \
    --bitcoind.rpchost=localhost:18443 --bitcoind.rpcuser=admin1 --bitcoind.rpcpass=123 \
    --bitcoind.zmqpubrawblock=tcp://127.0.0.1:29000 --bitcoind.zmqpubrawtx=tcp://127.0.0.1:29001 \
    --noseedbackup --tlskeypath=/tmp/lnd-regtest-$i/tls.key --tlscertpath=/tmp/lnd-regtest-$i/tls.cert \
    --rpclisten=0.0.0.0:$rpcport --norest \
    --logdir=/tmp/lnd-regtest-$i/logs \
    --externalip=127.0.0.1:$listenport \
    --listen=0.0.0.0:$listenport \
    --protocol.wumbo-channels \
    --configfile=/tmp/lnd-regtest-$1/lnd.conf > /dev/null 2>&1 &
    # shellcheck disable=SC2139 disable=SC2086
    alias lncli-$i="$LNCLI --lnddir=/tmp/lnd-regtest-$i --network regtest --rpcserver=localhost:$rpcport"
    alias lnd-$i-logs="tail -f /tmp/l$i-$network/log"
  done
}

start_peerswap_lnd() {
  if [ -z "$1" ]; then
      node_count=2
    else
      node_count=$1
  fi
    for i in $(seq $node_count); do
      lndrpcport=$((10101 + i * 100))
      listenport=$((42069 + i * 100))
      lndpath="/tmp/lnd-regtest-$i"
      mkdir -p "/tmp/lnd-peerswap-$i/"
      cat <<-EOF >"/tmp/lnd-peerswap-$i/config"
network=regtest
host=localhost:$listenport
datadir=/tmp/lnd-peerswap-$i/
lnd.host=localhost:$lndrpcport
lnd.tlscertpath=$lndpath/tls.cert
lnd.macaroonpath=$lndpath/data/chain/bitcoin/regtest/admin.macaroon
bitcoinswaps=true
liquid.rpcuser=admin1
liquid.rpcpass=123 
liquid.rpchost=http://127.0.0.1
liquid.rpcport=18884
liquid.rpcwallet=swaplnd-$i
accept_all_peers=true
EOF
  
    ./out/peerswapd "--configfile=/tmp/lnd-peerswap-$i/config" > /dev/null 2>&1 &

    done
}
start_nodes() {
  LIGHTNINGD='lightningd'
  if [ -z "$1" ]; then
    node_count=2
  else
    node_count=$1
  fi
  if [ "$node_count" -gt 100 ]; then
    node_count=100
  fi
  if [ -z "$2" ]; then
    network=regtest
  else
    network=$2
  fi
  LN_NODES=$node_count
  for i in $(seq $node_count); do
    socket=$((7070 + i * 101))
    liquidrpcPort=18884
    mkdir -p "/tmp/l$i-$network"
    # Node config
    cat <<-EOF >"/tmp/l$i-$network/config"
network=$network
log-level=debug
log-file=/tmp/l$i-$network/log
addr=127.0.0.1:$socket
bitcoin-rpcuser=admin1
bitcoin-rpcpassword=123
bitcoin-rpcconnect=127.0.0.1
bitcoin-rpcport=18443
large-channels
EOF
    # If we've configured to use developer, add dev options
    if $LIGHTNINGD --help | grep -q dev-fast-gossip; then
      cat <<-EOF >>"/tmp/l$i-$network/config"
dev-fast-gossip
dev-bitcoind-poll=5
experimental-dual-fund
funder-policy=match
funder-policy-mod=100
funder-min-their-funding=10000
funder-per-channel-max=100000
funder-fuzz-percent=0
EOF
    fi
    PWD=$(pwd)
    # Start the lightning nodes
    test -f "/tmp/l$i-$network/lightningd-$network.pid" ||
      "$LIGHTNINGD" "--lightning-dir=/tmp/l$i-$network" --daemon \
        "--plugin=$PWD/out/peerswap-plugin" \
        --peerswap-elementsd-rpchost=http://127.0.0.1 \
        --peerswap-elementsd-rpcport=$liquidrpcPort \
        --peerswap-elementsd-rpcuser=admin1 \
        --peerswap-elementsd-rpcpassword=123 \
        --peerswap-elementsd-rpcwallet=swap-$i \
        --peerswap-policy-path=/tmp/l$i-$network/policy.conf
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-cli="$LCLI --lightning-dir=/tmp/l$i-$network"
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-log="less /tmp/l$i-$network/log"
    alias l$i-follow="tail -f /tmp/l$i-$network/log"
    alias l$i-followf="tail -f /tmp/l$i-$network/log | grep peerswap"
  done
  # set peer allowlist in policy
  for i in $(seq $node_count); do
    POLICY="/tmp/l$i-$network/policy.conf"
    if [ ! -f "$POLICY" ]; then
      for j in $(seq $node_count); do
        cat <<-EOF >> $POLICY
allowlisted_peers=$($LCLI --lightning-dir=/tmp/l$j-$network getinfo | jq -r .id)
EOF
      done
      echo "accept_all_peers=1" >> $POLICY
    fi
  done
  # Give a hint.
  echo "Commands: "
  for i in $(seq $node_count); do
    echo "	l$i-cli, l$i-log, l$i-follow, l$i-followf"
  done
}
remove_swaps() {
  rm /tmp/l1-regtest/regtest/swaps/swaps
  rm /tmp/l2-regtest/regtest/swaps/swaps
}
setup_alias() {
  if [ -z "$1" ]; then
    node_count=2
  else
    node_count=$1
  fi
  if [ -z "$2" ]; then
    network=regtest
  else
    network=$2
  fi
  LN_NODES=$node_count

  LCLI='lightning-cli'
  LNCLI='lncli'
  for i in $(seq $node_count); do
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-cli="$LCLI --lightning-dir=/tmp/l$i-$network"
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-log="less /tmp/l$i-$network/log"
    alias l$i-follow="tail -f /tmp/l$i-$network/log"
    alias l$i-followf="tail -f /tmp/l$i-$network/log | grep peerswap"
  done
  for i in $(seq 3); do
    rpcport=$((10101 + i * 100))
    alias lncli-$i="$LNCLI --lnddir=/tmp/lnd-regtest-$i --network regtest --rpcserver=localhost:$rpcport"
    alias lnd-$i-logs="tail -f /tmp/lnd-regtest-$i/logs/bitcoin/regtest/lnd.log"
  done
  # Give a hint.
  echo "Commands: "
  for i in $(seq $node_count); do
    echo "	l$i-cli, l$i-log, l$i-follow, l$i-followf, lncli-$i, lnd-$i-logs"
  done

  alias bt-cli='bitcoin-cli -regtest -rpcuser=admin1 -rpcpassword=123 -rpcconnect=127.0.0.1 -rpcport=18443'
  alias et-cli='elements-cli -rpcuser=admin1 -rpcpassword=123 -rpcconnect=127.0.0.1 -rpcport=18884'
  alias et-cli2='elements-cli -rpcuser=admin1 -rpcpassword=123 -rpcconnect=127.0.0.1 -rpcport=18885'

}

connect_nodes() {
  # connect clightning nodes
  L2_PORT=$(l2-cli getinfo | jq .binding[0].port)

  L2_PUBKEY=$(l2-cli getinfo | jq -r .id)

  L2_CONNECT="$L2_PUBKEY@127.0.0.1:$L2_PORT"

  echo "$(l1-cli connect $L2_CONNECT)"
  # connect lnd ndoes
  LND_URI1=$(lncli-1 getinfo | jq -r .uris[0])
  LND_URI2=$(lncli-2 getinfo | jq -r .uris[0])

  echo "$(lncli-1 connect $LND_URI2)"
  echo "$(l1-cli connect $LND_URI1)"
  echo "$(l1-cli connect $LND_URI2)"
  echo "$(l2-cli connect $LND_URI1)"
  echo "$(l2-cli connect $LND_URI2)"
}
rebuild() {
  make build
  restart
}
restart() {
  stop_nodes "$1" regtest
  start_nodes "$nodes" regtest
  start_nodes_lnd "$nodes"
}

l1-pay() {
  LABEL=$(tr -dc A-Za-z0-9 </dev/urandom | head -c 13)
  BOLT11=$(l1-cli invoice $1 $LABEL "foo" | jq -r .bolt11)
  echo $BOLT11
  RES=$(l2-cli pay $BOLT11)
  echo $RES
}
l2-pay() {
  LABEL=$(tr -dc A-Za-z0-9 </dev/urandom | head -c 13)
  BOLT11=$(l2-cli invoice $1 $LABEL "foo" | jq -r .bolt11)
  RES=$(l1-cli pay $BOLT11)
  echo $RES
}

setup_channel() {
  connect_nodes
  L2_PUBKEY=$(l2-cli getinfo | jq -r .'id')
  echo $(l1-cli fundchannel $L2_PUBKEY 10000000)
  echo $(generate 12)
}

setup_channel_lnd() {
  connect_nodes
  L2_PUBKEY=$(lncli-2 getinfo | jq -r .'identity_pubkey')
  echo $(lncli-1 openchannel $L2_PUBKEY 1000000)
  echo $(generate 12)
}

setup_channel_lnd_cl() {
  connect_nodes
  L1_PUBKEY=$(l1-cli getinfo | jq -r .'id')
  echo $(lncli-1 openchannel $L1_PUBKEY 1000000)
  echo $(generate 12)
}

fund_node() {
  L1_ADDR=$(l1-cli newaddr | jq .'bech32')
  L1_ADDR=$(sed -e 's/^"//' -e 's/"$//' <<<"$L1_ADDR")
  echo $(bt-cli generatetoaddress 1 $L1_ADDR)
  echo $(generate 100)
}

fund_node_2() {
  L1_ADDR=$(l2-cli newaddr | jq .'bech32')
  L1_ADDR=$(sed -e 's/^"//' -e 's/"$//' <<<"$L1_ADDR")
  echo $(bt-cli generatetoaddress 1 $L1_ADDR)
  echo $(generate 100)
}

fund_node_lnd() {
  L1_ADDR=$(lncli-1 newaddress p2wkh | jq -r .'address')
  echo $(bt-cli generatetoaddress 1 $L1_ADDR)
  echo $(generate 100)
}

fund_node_lnd_2() {
  L1_ADDR=$(lncli-2 newaddress p2wkh | jq -r .'address')
  echo $(bt-cli generatetoaddress 1 $L1_ADDR)
  echo $(generate 100)
}



fund_nodes_l() {
  echo $(l1-cli dev-liquid-faucet)
  echo $(l2-cli dev-liquid-faucet)
}

l_generate() {
  if [ -z "$1" ]; then
    block_count=1
  else
    block_count=$1
  fi
  res=$(et-cli generatetoaddress $block_count ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep)
  echo $res
}

generate() {
  if [ -z "$1" ]; then
    block_count=1
  else
    block_count=$1
  fi
  res=$(bt-cli generatetoaddress $block_count 2NDsRVXmnw3LFZ12rTorcKrBiAvX54LkTn1)
  echo $res
}

reset_dev_env() {
  remove_nodes
  stop_docker_env
  rm -rf .ci/docker/config/regtest
  rm -rf .ci/docker/liquid-config/liquidregtest
  rm -rf .ci/docker/liquid-config2/liquidregtest
}

start_dev_env() {
  start_docker_env
  rebuild
}

stop_peerswap_lnd() {
  ./out/pscli "--rpcserver=localhost:42169" stop
  ./out/pscli "--rpcserver=localhost:42269" stop
}

rebuild_peerswap_lnd() {
  stop_peerswap_lnd
  make build
  start_peerswap_lnd
}