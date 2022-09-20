#!/bin/bash

# Cln
LIGHTNINGD="lightningd"
LIGHTNING_CLI="lightning-cli"

# Lnd
LNDD="lnd"
LND_CLI="lncli"

# Liquid
LIQUID_RPC_PORT=18884

# Policy config
ACCEPT_ALL_PEERS=1

# Aliases
CLN_CLI_BASE_ALIAS="lightning-cli"
LND_CLI_BASE_ALIAS="lncli"

start_docker_env() {
  docker-compose -f .ci/docker/docker-compose.yml up -d --remove-orphans
}

stop_docker_env() {
  docker-compose -f .ci/docker/docker-compose.yml down
}

prefixwith() {
  local prefix="${1}"
  shift
  "$@" > >(sed "s/^/[$(date -u) ${prefix}]: /") 2> >(sed "s/^/[$(date -u) ${prefix}]: /" >&2)
}

# Starts a new cln node with arguments [id] [chain]. If the LIGHTNINGD was build
# with developer-enabled flag, the config uses a faster bitcoin-poll.
start_cln_node() {
  if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local id=${1}
    local prefix="cln-${id}"
    local network="regtest"
    local addr="127.0.0.1:$((7070 + ${id} * 101))"
    if [ -z ${2} ]; then
    else
      network=${2}
    fi
    prefixwith $prefix echo "creating cln node on network ${network}, listening on ${addr}"
    local dir="/tmp/test-peerswap/cln-${id}-${network}"
    prefixwith $prefix echo "creating node dir ${dir}"
    mkdir -p ${dir}

    # Write config file
    touch ${dir}/config
    cat <<-EOF >"${dir}/config"
network=${network}
log-level=debug
log-file=${dir}/log
addr=${addr}
bitcoin-rpcuser=admin1
bitcoin-rpcpassword=123
bitcoin-rpcconnect=127.0.0.1
bitcoin-rpcport=18443
large-channels
EOF

    # If we've configured to use developer we append dev options.
    if $LIGHTNINGD --help | grep -q dev-fast-gossip; then
      prefixwith $prefix echo "using a developer-enabled cln node"
      cat <<-EOF >>"${dir}/config"
dev-fast-gossip
dev-bitcoind-poll=1
experimental-dual-fund
funder-policy=match
funder-policy-mod=100
funder-min-their-funding=10000
funder-per-channel-max=100000
funder-fuzz-percent=0
EOF
    fi

    # Write policy config
    prefixwith $prefix echo "writing policy file"
    touch ${dir}/policy.conf
    echo "accept_all_peers=${ACCEPT_ALL_PEERS}" >> ${dir}/policy.conf

    # Start node
    prefixwith $prefix echo "starting node"
    if [ -f "${dir}/lightningd-${network}.pid" ]; then
      prefixwith $prefix echo "${LIGHTNINGD} is already running with pid $(cat ${dir}/lightningd-${network}.pid)"
      return 1
    else
      ${LIGHTNINGD}\
      --lightning-dir="${dir}" \
      --daemon \
      --plugin="$(pwd)/out/peerswap-plugin" \
      --peerswap-elementsd-rpchost="http://127.0.0.1" \
      --peerswap-elementsd-rpcport="${LIQUID_RPC_PORT}" \
      --peerswap-elementsd-rpcuser=admin1 \
      --peerswap-elementsd-rpcpassword=123 \
      --peerswap-elementsd-rpcwallet="swap-${id}" \
      --peerswap-policy-path="${dir}/policy.conf"
      if [ $? -eq 1 ]; then 
        prefixwith $prefix echo "cln node crashed"
        rm ${dir}/lightningd-${network}.pid
        return 1
      fi
    fi
  alias ${CLN_CLI_BASE_ALIAS}-${id}="${LIGHTNING_CLI} --lightning-dir=${dir}"
  alias cln-log-${id}="less ${dir}/log"
  alias cln-logf-${id}="tail -f ${dir}/log"
  echo "\nCommands:\n${CLN_CLI_BASE_ALIAS}-${id}, cln-log-${id}, cln-logf-${id}\n"
  fi
}

# Stops a cln node with arguments [id] [chain]. 
stop_cln_node() {
  if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local id=${1}
    local prefix="cln-${id}"
    local network="regtest"
    if [ -z ${2} ]; then
    else
      network=${2}
    fi
    local dir="/tmp/test-peerswap/cln-${id}-${network}"  
    if ! [ -f "${dir}/lightningd-${network}.pid" ]; then
      prefixwith $prefix echo "no running cln node found"
      return
    else
      local pid=$(cat ${dir}/lightningd-${network}.pid)
      prefixwith $prefix echo "killing cln node with pid ${pid}"
      kill $pid
      unalias ${CLN_CLI_BASE_ALIAS}-${id}
      rm "${dir}/lightningd-${network}.pid"
    fi
  fi
}

# Removes the temporary dir for a node with arguments [id] [chain]. Stops the
# node before trying to remove the dir.
remove_cln_node() {
   if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local id=${1}
    local prefix="cln-${id}"
    local network="regtest"
    if [ -z ${2} ]; then
    else
      network=${2}
    fi
    local dir="/tmp/test-peerswap/cln-${id}-${network}"
    stop_cln_node ${1}
    if [ -d ${dir} ]; then 
      prefixwith ${prefix} echo "removing node dir ${dir}"
      rm -rf ${dir}
      return
    fi
  fi
}

# Setup of a linear cln network with the following topology:
# (Alice) 0.1 Btc ---------- 0 Btc (Bob).
setup_cln_network() {
  local n_nodes=2
  local network="regtest"
  if [ -z ${1} ]; then
  else
    network=${1}
  fi
  CLN_SETUP_NETWORK=$network
  local prefix="cln-${CLN_SETUP_NETWORK}"
  prefixwith $prefix echo "Setting up a cln ${CLN_SETUP_NETWORK} network"

  # Generate blocks
  generate 10

  # Create and start nodes
  for i in $(seq $n_nodes); do
    start_cln_node ${i} $CLN_SETUP_NETWORK
    fund_cln_node $i
  done
  generate 12
  
  # Connect nodes
  prefixwith $prefix echo "Connecting nodes"
  local to=$(eval ${LIGHTNING_CLI}-2 getinfo | jq -r '"\(.id)@\(.binding[0].address):\(.binding[0].port)"')
  eval ${LIGHTNING_CLI}-1 connect $to

  # Await blockchain sync
  local is_synced=false
  local blockcount=$(bitcoin-cli \
    -chain="${CLN_SETUP_NETWORK}" \
    -rpcuser=admin1 \
    -rpcpassword=123 \
    -rpcconnect=127.0.0.1 \
    -rpcport=18443 \
    getblockcount)
  while; do
    blockheight1=$(eval ${LIGHTNING_CLI}-1 getinfo | jq -r .'blockheight')
    blockheight2=$(eval ${LIGHTNING_CLI}-2 getinfo | jq -r .'blockheight')
    if [ $blockheight1 -ge $blockcount ] && [ $blockheight2 -ge $blockcount ]; then
      prefixwith $prefix echo "Nodes are synced to blockchain"  
      break
    fi
    prefixwith $prefix echo "Waiting for nodes to be synced to blockchain"
    sleep 5  
  done

  # Fund channel
  to=$(eval ${LIGHTNING_CLI}-2 getinfo | jq -r .'id')
  eval ${LIGHTNING_CLI}-1 fundchannel $to 10000000
  generate 12

  # Await channel active
  while; do
    local state=$(eval ${LIGHTNING_CLI}-1 listfunds | jq -r '.channels[0].state')
    if [ "$state" = "CHANNELD_NORMAL" ]; then
      prefixwith $prefix echo "Channel is active"
      break
    fi
    prefixwith $prefix echo "Waiting for channel to be active"
    sleep 5
  done
}

# Stops a running cln network. Uses the global $CLN_SETUP_NETWORK to determain
# the network.
stop_cln_network() {
  local n_nodes=2
  
  # Stop nodes
  for i in $(seq $n_nodes); do
    stop_cln_node ${i} $CLN_SETUP_NETWORK
  done  
}

# Restarts a running cln network. Uses the global $CLN_SETUP_NETWORK to 
# determain the network.
restart_cln_network() {
    local n_nodes=2
    
    stop_cln_network
    for i in $(seq $n_nodes); do
      start_cln_node ${i} $CLN_SETUP_NETWORK
    done
}

# Builds a clean set of peerswap bins. After building the bins, restarts a 
# running cln network to make use of the bins. Uses the global 
# $CLN_SETUP_NETWORK to determain the network.
rebuild_cln_network() {
  make clean-bins
  make bins
  restart_cln_network
}

# Removes a cln network. Deletes the temporary dirs of the nodes.
remove_cln_network() {
  local n_nodes=2
  prefixwith "cln-${CLN_SETUP_NETWORK}" echo "Tear down cln ${CLN_SETUP_NETWORK}"
  for i in $(seq $n_nodes); do
    remove_cln_node $i $CLN_SETUP_NETWORK
  done
}

# Funds the cln node with the argument [id] uses the global #CLN_SETUP_NETWORK
# to determain the network. Funds 1Btc to a fresh address on the node.
fund_cln_node() {
  if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local to=$(eval ${LIGHTNING_CLI}-${1} newaddr | jq -r .'bech32')
    bitcoin-cli \
    -chain="${CLN_SETUP_NETWORK}" \
    -rpcuser=admin1 \
    -rpcpassword=123 \
    -rpcconnect=127.0.0.1 \
    -rpcport=18443 \
    sendtoaddress $to 1
  fi
}

# LND
# Starts a new lnd node with arguments [id] [network].
start_lnd_node() {
  if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local id=${1}
    local prefix="lnd-${id}"
    local network="regtest"
    local rpc="127.0.0.1:$((10101 + ${id} * 101))"
    local listen="127.0.0.1:$((10102 + ${id} * 101))"
    if [ -z ${2} ]; then
    else
      network=${2}
    fi
    prefixwith $prefix echo "creating lnd node on network ${network}, rpc on ${rpc}"
    local dir="/tmp/test-peerswap/lnd-${id}-${network}"
    prefixwith $prefix echo "creating node dir ${dir}"
    mkdir -p ${dir}/data

    # Write config file
    touch ${dir}/data/lnd.conf

     cat <<-EOF >"${dir}/data/lnd.conf"
[Application Options]
datadir=${dir}/data
logdir=${dir}/logs
tlskeypath=${dir}/data/tls.key
tlscertpath=${dir}/data/tls.cert
rpclisten=${rpc}
listen=${listen}
norest=true
noseedbackup=true
externalip=${listen}

[protocol]
protocol.wumbo-channels=true

[bitcoin]
bitcoin.active=true
bitcoin.${network}=true
bitcoin.node=bitcoind

[Bitcoind]
bitcoind.rpchost=localhost:18443
bitcoind.rpcuser=admin1
bitcoind.rpcpass=123
bitcoind.zmqpubrawblock=tcp://127.0.0.1:29000
bitcoind.zmqpubrawtx=tcp://127.0.0.1:29001
EOF

    # Start node
    if [ -f "${dir}/lnd-${network}.pid" ]; then
      prefixwith $prefix echo "${LIGHTNINGD} is already running with pid $(cat ${dir}/lnd-${network}.pid)"
      return 1
    else

      ${LNDD} --configfile=${dir}/data/lnd.conf > /dev/null 2>&1 &
      local pid=$!

      if [ $? -eq 1 ]; then 
        prefixwith $prefix echo "lnd node crashed on startup"
        return 1
      fi

      touch ${dir}/lnd-${network}.pid
      echo "${pid}" > ${dir}/lnd-${network}.pid
    fi
    alias ${LND_CLI_BASE_ALIAS}-${id}="${LND_CLI} \
      --lnddir=${dir}/data \
      --network=${network} \
      --chain=bitcoin \
      --rpcserver=${rpc} \
      --tlscertpath=${dir}/data/tls.cert \
      --macaroonpath=${dir}/data/chain/bitcoin/${network}/admin.macaroon"
    alias lnd-log-${id}="less ${dir}/logs/bitcoin/${network}/lnd.log"
    alias lnd-logf-${id}="tail -f ${dir}/logs/bitcoin/${network}/lnd.log"
    echo "\nCommands:\n${LND_CLI_BASE_ALIAS}-${id}, lnd-log-${id}, lnd-logf-${id}\n"
  fi
}

# Stops a lnd node with arguments [id] [chain]. 
stop_lnd_node() {
  if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local id=${1}
    local prefix="lnd-${id}"
    local network="regtest"
    if [ -z ${2} ]; then
    else
      network=${2}
    fi
    local dir="/tmp/test-peerswap/lnd-${id}-${network}"  
    if ! [ -f "${dir}/lnd-${network}.pid" ]; then
      prefixwith $prefix echo "no running lnd node found"
      return
    else
      local pid=$(cat ${dir}/lnd-${network}.pid)
      prefixwith $prefix echo "killing lnd node with pid ${pid}"
      kill $pid
      unalias ${LND_CLI_BASE_ALIAS}-${id}
      rm "${dir}/lnd-${network}.pid"
    fi
  fi
}

# Removes the temporary dir for a lnd node with arguments [id] [chain]. Stops 
# the node before trying to remove the dir.
remove_lnd_node() {
   if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local id=${1}
    local prefix="lnd-${id}"
    local network="regtest"
    if [ -z ${2} ]; then
    else
      network=${2}
    fi
    local dir="/tmp/test-peerswap/lnd-${id}-${network}"
    stop_lnd_node ${1} ${network}
    if [ -d ${dir} ]; then 
      prefixwith ${prefix} echo "removing node dir ${dir}"
      rm -rf ${dir}
      return
    fi
  fi
}

# Setup of a linear lnd network with the following topology:
# (Alice) 0.1 Btc ---------- 0 Btc (Bob).
setup_lnd_network() {
  local n_nodes=2
  local network="regtest"
  if [ -z ${1} ]; then
  else
    network=${1}
  fi
  LND_SETUP_NETWORK=$network
  local prefix="lnd-${LND_SETUP_NETWORK}"
  prefixwith $prefix echo "Setting up a lnd ${LND_SETUP_NETWORK} network"

  # Generate blocks
  generate 10 $LND_SETUP_NETWORK

  # Create and start nodes
  for i in $(seq $n_nodes); do
    start_lnd_node ${i} $LND_SETUP_NETWORK
  done

  # Connect nodes
  while; do
    prefixwith $prefix echo "Connecting nodes"
    local to=$(eval ${LND_CLI}-2 getinfo | jq -r '.uris[0]')
    eval ${LND_CLI}-1 connect $to > /dev/null
    if [ $? -eq 0 ]; then 
      break
    fi
    sleep 5
  done

  # Fund nodes
  for i in $(seq $n_nodes); do
    fund_lnd_node $i
  done
  generate 12 $LND_SETUP_NETWORK

  # Await blockchain sync
  local blockcount=$(bitcoin-cli \
    -chain="${LND_SETUP_NETWORK}" \
    -rpcuser=admin1 \
    -rpcpassword=123 \
    -rpcconnect=127.0.0.1 \
    -rpcport=18443 \
    getblockcount)
  while; do
    blockheight1=$(eval ${LND_CLI}-1 getinfo | jq -r .'block_height')
    blockheight2=$(eval ${LND_CLI}-2 getinfo | jq -r .'block_height')
    if [ $blockheight1 -ge $blockcount ] && [ $blockheight2 -ge $blockcount ]; then
      prefixwith $prefix echo "Nodes are synced to blockchain"  
      break
    fi
    prefixwith $prefix echo "Waiting for nodes to be synced to blockchain"
    sleep 5  
  done

  # Fund channel
  to=$(eval ${LND_CLI}-2 getinfo | jq -r .'identity_pubkey')
  eval ${LND_CLI}-1 openchannel $to 10000000
  generate 12 $LND_SETUP_NETWORK

  # Await channel active
  while; do
    local active=$(eval ${LND_CLI}-1 listchannels | jq -r '.channels[0].active')
    if [ "$active" = "true" ]; then
      prefixwith $prefix echo "Channel is active"
      break
    fi
    prefixwith $prefix echo "Waiting for channel to be active"
    sleep 5
  done
}

# Restarts a running lnd network. Uses the global $LND_SETUP_NETWORK to 
# determain the network.
restart_lnd_network() {
    local n_nodes=2
    
    stop_lnd_network
    for i in $(seq $n_nodes); do
      start_lnd_node ${i} $LND_SETUP_NETWORK
    done
}

# Builds a clean set of peerswap bins. After building the bins, restarts a 
# running lnd network to make use of the bins. Uses the global 
# $LND_SETUP_NETWORK to determain the network.
rebuild_lnd_network() {
  make clean-bins
  make bins
  restart_lnd_network
}

# Removes a lnd network. Deletes the temporary dirs of the nodes.
remove_lnd_network() {
  local n_nodes=2
  prefixwith "cln-${LND_SETUP_NETWORK}" echo "Tear down lnd ${LND_SETUP_NETWORK}"
  for i in $(seq $n_nodes); do
    remove_lnd_node $i $LND_SETUP_NETWORK
  done
}

# Funds the lnd node with the argument [id] [chain]. Funds 1Btc to a fresh 
# address on the node.
fund_lnd_node() {
  if [ -z ${1} ]; then
    echo "missing node id"
    return 1
  else
    local network="regtest"
    if [ -z ${2} ]; then
    else
      network=${2}
    fi
    local to=$(eval ${LND_CLI}-${1} newaddress p2wkh | jq -r .'address')
    bitcoin-cli \
    -chain=${network} \
    -rpcuser=admin1 \
    -rpcpassword=123 \
    -rpcconnect=127.0.0.1 \
    -rpcport=18443 \
    sendtoaddress $to 1
  fi
}

start_peerswap_lnd() {
  if [ -z "$1" ]; then
      node_count=2
    else
      node_count=$1
  fi
    for i in $(seq $node_count); do
      lndrpcport=$((10101 + i * 100))
      listenport=$((42069 + i * 100)) alias l$i-cli="$LCLI --lightning-dir=/tmp/l$i-$network"
      restport=$((41069 + i * 100))
      lndpath="/tmp/lnd-regtest-$i"
      mkdir -p "/tmp/lnd-peerswap-$i/"
      cat <<-EOF >"/tmp/lnd-peerswap-$i/config"
network=regtest
host=localhost:$listenport
resthost=localhost:$restport
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

fund_nodes_l() {
  echo $(lightning-cli-1 dev-liquid-faucet)
  echo $(lightning-cli-1 dev-liquid-faucet)
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
  local network="regtest"
  if [ -z ${2} ]; then
  else
    network=${2}
  fi
  bitcoin-cli \
    -chain=${network} \
    -rpcuser=admin1 \
    -rpcpassword=123 \
    -rpcconnect=127.0.0.1 \
    -rpcport=18443 \
    generatetoaddress $block_count "2NDsRVXmnw3LFZ12rTorcKrBiAvX54LkTn1"
}

reset_dev_env() {
  remove_cln_network
  remove_lnd_network
  stop_docker_env
  rm -rf .ci/docker/config/regtest
  rm -rf .ci/docker/liquid-config/liquidregtest
  rm -rf .ci/docker/liquid-config2/liquidregtest
}

start_dev_env() {
  start_docker_env
  setup_cln_network
  setup_lnd_network
}

stop_peerswap_lnd() {
  ./out/pscli "--rpcserver=localhost:42169" stop
  ./out/pscli "--rpcserver=localhost:42269" stop
}

rebuild_peerswap_lnd() {
  stop_peerswap_lnd
  rm out/peerswapd
  rm out/pscli
  make bins
  start_peerswap_lnd
}