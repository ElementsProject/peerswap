let
  nix-bitcoin-release = pkgs.fetchFromGitHub {
      owner = "fort-nix";
      repo = "nix-bitcoin";
      rev = "v0.0.49";
      sha256 = "03b1zpwbcz98vrcnfflnxdcg17mhip3p63asp3jqzzzk8v4f1rna";
  };

  nix-bitcoin = pkgs.callPackage nix-bitcoin-release {};

  nixpkgs-unstable-path = (import "${toString nix-bitcoin-release}/pkgs/nixpkgs-pinned.nix").nixpkgs-unstable;
  nixpkgs-unstable = import nixpkgs-unstable-path { };

 pkgs = import <nixpkgs> {};
in
with pkgs;
    stdenv.mkDerivation rec {
     name = "peerswap-environment";
     # python packages python39Full python39Packages.pip python39Packages.bitcoinlib sqlite
     li = nixpkgs-unstable.clightning;

     buildInputs = [ docker-compose act bitcoin protoc-gen-go protoc-gen-go-grpc nixpkgs-unstable.elementsd li xonsh];
     path = lib.makeBinPath [  ];
     
     shellHook = ''

     alias lightning-cli='${li}/bin/lightning-cli'
     alias lightningd='${li}/bin/lightningd'
     alias bitcoind='${bitcoin}/bin/bitcoind'
     alias bitcoin-cli='${bitcoin}/bin/bitcoin-cli'

    start_docker_env() {
       docker-compose -f .ci/docker/docker-compose.yml up -d
    }

    stop_docker_env() {
        docker-compose -f .ci/docker/docker-compose.yml down
    }
    setup_pyln() {
        # Tells pip to put packages into $PIP_PREFIX instead of the usual locations.
        # See https://pip.pypa.io/en/stable/user_guide/#environment-variables.
        export PIP_PREFIX=$(pwd)/_build/pip_packages
        export PYTHONPATH="$PIP_PREFIX/${pkgs.python39Full.sitePackages}:$PYTHONPATH"
        export PATH="$PIP_PREFIX/bin:$PATH"
        unset SOURCE_DATE_EPOCH

        pip install pyln-testing
        pip install pyln-client
    }

    stop_nodes() {
        if [ -z "$2" ]; then
            network=regtest
        else
            network="$2"
        fi
        if [ -n "$LN_NODES" ]; then
            for i in $(seq $LN_NODES); do
                test ! -f "/tmp/l$i-$network/lightningd-$network.pid" || \
                    (kill "$(cat "/tmp/l$i-$network/lightningd-$network.pid")"; \
                    rm "/tmp/l$i-$network/lightningd-$network.pid")
                unalias "l$i-cli"
                unalias "l$i-log"
            done
        fi
    }

    remove_nodes() {
         stop_nodes
         rm -rf /tmp/l2-regtest/
         rm -rf /tmp/l1-regtest/
    }

        start_nodes() {
         alias lightning-cli='${li}/bin/lightning-cli'
         LIGHTNINGD='${li}/bin/lightningd'
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
    		socket=$(( 7070 + i * 101))
    		liquidrpcPort=$((18883 + i))
    		mkdir -p "/tmp/l$i-$network"
    		# Node config
    		cat <<- EOF > "/tmp/l$i-$network/config"
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
    			cat <<- EOF >> "/tmp/l$i-$network/config"
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
    		test -f "/tmp/l$i-$network/lightningd-$network.pid" || \
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
    	done
    	# Give a hint.
    	echo "Commands: "
    	for i in $(seq $node_count); do
    		echo "	l$i-cli, l$i-log,"
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

        LCLI='${li}/bin/lightning-cli'
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
            echo "	l$i-cli, l$i-log, l$i-follow"
        done
        alias e-cli="nigiri rpc --liquid"
        alias b-cli="nigiri rpc"

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


    l1-pay()  {
        LABEL=$(tr -dc A-Za-z0-9 </dev/urandom | head -c 13)
        BOLT11=$(l1-cli invoice $1 $LABEL "foo" | jq -r .bolt11)
        RES=$(l2-cli pay $BOLT11)
        echo $RES
    }
    l2-pay()  {
        LABEL=$(tr -dc A-Za-z0-9 </dev/urandom | head -c 13)
        BOLT11=$(l2-cli invoice $1 $LABEL "foo" | jq -r .bolt11)
        RES=$(l1-cli pay $BOLT11)
        echo $RES
    }


    setup_channel() {
        L2_PUBKEY=$(l2-cli getinfo | jq -r .id)
        L1_ADDR=$(l1-cli newaddr | jq .'bech32')
        L1_ADDR=$(sed -e 's/^"//' -e 's/"$//' <<<"$L1_ADDR")
        echo $(bt-cli generatetoaddress 1  $L1_ADDR)
        echo $(generate 200)
        echo $(l1-cli fundchannel $L2_PUBKEY 10000000)
    }

    fund_nodes() {
        l1_liquid=$(l1-cli liquid-wallet-getaddress)
        l1_liquid=$(sed -e 's/^"//' -e 's/"$//' <<<"$l1_liquid")
        echo $l1_liquid
        faucet $l1_liquid
    }

    faucet-l() {
        address=$(l1-cli liquid-wallet-getaddress)
        echo $address
        nigiri faucet --liquid $address 1
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

    # setup_pyln
    setup_alias
     '';

}