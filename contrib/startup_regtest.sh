#!/bin/bash

start_docker_env() {
  docker-compose -f .ci/docker/docker-compose.yml up -d
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
      if [ -d $LN_DIR ]; then
        echo "removing node from $LN_DIR"
        rm -rf $LN_DIR
      fi
    done
  fi
  unset LN_DIR
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
    if [ $i -le 2 ]; then
      liquidrpcPort=$((18883 + i))
    fi
    mkdir -p "/tmp/l$i-$network"
    # Node config
    cat <<-EOF >"/tmp/l$i-$network/config"
network=$network
log-level=debug
log-file=/tmp/l$i-$network/log
addr=localhost:$socket
bitcoin-rpcuser=admin1
bitcoin-rpcpassword=123
bitcoin-rpcconnect=localhost
bitcoin-rpcport=18443
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
        "--plugin=$PWD/peerswap" \
        --peerswap-liquid-rpchost=http://localhost \
        --peerswap-liquid-rpcport=$liquidrpcPort \
        --peerswap-liquid-rpcuser=admin1 \
        --peerswap-liquid-rpcpassword=123 \
        --peerswap-liquid-network=regtest \
        --peerswap-liquid-rpcwallet=swap-$i
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-cli="$LCLI --lightning-dir=/tmp/l$i-$network"
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-log="less /tmp/l$i-$network/log"
    alias l$i-follow="tail -f /tmp/l$i-$network/log"
    alias l$i-followf="tail -f /tmp/l$i-$network/log | grep peerswap"
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
  for i in $(seq $node_count); do
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-cli="$LCLI --lightning-dir=/tmp/l$i-$network"
    # shellcheck disable=SC2139 disable=SC2086
    alias l$i-log="less /tmp/l$i-$network/log"
    alias l$i-follow="tail -f /tmp/l$i-$network/log"
    alias l$i-followf="tail -f /tmp/l$i-$network/log | grep peerswap"
  done
  # Give a hint.
  echo "Commands: "
  for i in $(seq $node_count); do
    echo "	l$i-cli, l$i-log, l$i-follow, l$i-followf"
  done

  alias bt-cli='bitcoin-cli -regtest -rpcuser=admin1 -rpcpassword=123 -rpcconnect=localhost -rpcport=18443'
  alias et-cli='elements-cli -rpcuser=admin1 -rpcpassword=123 -rpcconnect=localhost -rpcport=18884'
  alias et-cli2='elements-cli -rpcuser=admin1 -rpcpassword=123 -rpcconnect=localhost -rpcport=18885'

}

connect_nodes() {
  L2_PORT=$(l2-cli getinfo | jq .binding[0].port)

  L2_PUBKEY=$(l2-cli getinfo | jq -r .id)

  L2_CONNECT="$L2_PUBKEY@127.0.0.1:$L2_PORT"

  echo $(l1-cli connect $L2_CONNECT)
}
rebuild() {
  make build
  restart
}
restart() {
  stop_nodes "$1" regtest
  start_nodes "$nodes" regtest
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
  L2_PUBKEY=$(l2-cli getinfo | jq -r .id)
  echo $(l1-cli fundchannel $L2_PUBKEY 10000000)
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
