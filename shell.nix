let
 pkgs = import <nixpkgs> {};
in
with pkgs;
    stdenv.mkDerivation rec {
     name = "sugarmama-environment";

     path = lib.makeBinPath [  ];
     
     shellHook = ''
    if [ -z "$PATH_TO_LIGHTNING" ] && [ -x cli/lightning-cli ] && [ -x lightningd/lightningd ]; then
        PATH_TO_LIGHTNING=$(pwd)
    fi


    if [ -z "$PATH_TO_LIGHTNING" ]; then
        # Already installed maybe?  Prints
        # shellcheck disable=SC2039
        type lightning-cli || return
        # shellcheck disable=SC2039
        type lightningd || return
        LCLI=lightning-cli
        LIGHTNINGD=lightningd
    else
        LCLI="$PATH_TO_LIGHTNING"/cli/lightning-cli
        LIGHTNINGD="$PATH_TO_LIGHTNING"/lightningd/lightningd
        # This mirrors "type" output above.
        echo lightning-cli is "$LCLI"
        echo lightningd is "$LIGHTNINGD"
    fi

    if [ -z "$PATH_TO_BITCOIN" ]; then
        if [ -d "$HOME/.bitcoin" ]; then
            PATH_TO_BITCOIN="$HOME/.bitcoin"
        elif [ -d "$HOME/Library/Application Support/Bitcoin/" ]; then
            PATH_TO_BITCOIN="$HOME/Library/Application Support/Bitcoin/"
        else
            echo "\$PATH_TO_BITCOIN not set to a .bitcoin dir?" >&2
            return
        fi
    fi

    if [ -z "$PATH_TO_ELEMENTS" ]; then
		if [ -d "$HOME/.elements" ]; then
			PATH_TO_ELEMENTS="$HOME/.elements"
		else
			echo "\$PATH_TO_ELEMENTS not set to a .elements dir" >&2
			return
		fi
	fi


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
    start_nigiri_env() {
        start_nodes "$nodes" regtest
    }

    stop_nigiri_env() {
        stop_nodes "$1" regtest
        unset LN_NODES
    }
    start_test_env() {

        test -f "$PATH_TO_ELEMENTS/liquid-regtest/bitcoin.pid" || \
		elementsd -chain=liquid-regtest -printtoconsole -logtimestamps -nolisten -validatepegin=0 -con_blocksubsidy=5000000000 -daemon

        # Wait for it to start.
        while ! elements-cli -chain=liquid-regtest ping 2> /tmp/null; do echo "awaiting elementsd..." && sleep 1; done

        # Kick it out of initialblockdownload if necessary
        if elements-cli -chain=liquid-regtest getblockchaininfo | grep -q 'initialblockdownload.*true'; then
            elements-cli -chain=liquid-regtest generatetoaddress 1 "$(elements-cli -chain=liquid-regtest getnewaddress)" > /dev/null
        fi
        alias et-cli='elements-cli -chain=liquid-regtest'

        # Start bitcoind in the background
        test -f "$PATH_TO_BITCOIN/regtest/bitcoind.pid" || \
            bitcoind -regtest -txindex -fallbackfee=0.00000253 -daemon

        # Wait for it to start.
        while ! bitcoin-cli -regtest ping 2> /tmp/null; do echo "awaiting bitcoind..." && sleep 1; done

        # Kick it out of initialblockdownload if necessary
        if bitcoin-cli -regtest getblockchaininfo | grep -q 'initialblockdownload.*true'; then
            # Modern bitcoind needs createwallet
            bitcoin-cli -regtest createwallet default >/dev/null 2>&1
            bitcoin-cli -regtest generatetoaddress 1 "$(bitcoin-cli -regtest getnewaddress)" > /dev/null
        fi
        alias bt-cli='bitcoin-cli -regtest'

        if [ -z "$1" ]; then
            nodes=2
        else
            nodes="$1"
        fi
        start_nodes "$nodes" regtest
        echo "	bt-cli, stop_ln"
        echo "	et-cli, stop_elem"

        
        connect_nodes
        setup_channel
    }
    remove_nodes() {
         rm -rf /tmp/l2-regtest/
         rm -rf /tmp/l1-regtest/
    }
    stop_test_env() {
        stop_nodes "$1" regtest
        test ! -f "$PATH_TO_BITCOIN/regtest/bitcoind.pid" || \
            (kill "$(cat "$PATH_TO_BITCOIN/regtest/bitcoind.pid")"; \
            rm "$PATH_TO_BITCOIN/regtest/bitcoind.pid")

        unset LN_NODES
        unalias bt-cli

        test ! -f "$PATH_TO_ELEMENTS/liquid-regtest/bitcoind.pid" || \
            (kill "$(cat "$PATH_TO_ELEMENTS/liquid-regtest/bitcoind.pid")"; \
            rm "$PATH_TO_ELEMENTS/liquid-regtest/bitcoind.pid")

        unalias et-cli
    }
    start_nodes() {
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
		mkdir -p "/tmp/l$i-$network"
		# Node config
		cat <<- EOF > "/tmp/l$i-$network/config"
		network=$network
		log-level=debug
		log-file=/tmp/l$i-$network/log
		addr=localhost:$socket
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
			bitcoin-rpcuser=admin1
			bitcoin-rpcpassword=123
			bitcoin-rpcconnect=localhost
			bitcoin-rpcport=18433
			EOF
		fi


		# Start the lightning nodes
		test -f "/tmp/l$i-$network/lightningd-$network.pid" || \
			"$LIGHTNINGD" "--lightning-dir=/tmp/l$i-$network" "--plugin=/mnt/c/Users/kon-dev/Documents/coding/liquid-swap/liquid-swap" --daemon --esplora-url=gude
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

	    for i in $(seq $node_count); do
	    # shellcheck disable=SC2139 disable=SC2086
        		alias l$i-cli="$LCLI --lightning-dir=/tmp/l$i-$network"
        		# shellcheck disable=SC2139 disable=SC2086
        		alias l$i-log="less /tmp/l$i-$network/log"
        		alias l$i-follow="tail -f /tmp/l$i-$network/log"
        		alias l$i-followf="tail -f /tmp/l$i-$network/log | grep liquid-swap"
        done
        # Give a hint.
        echo "Commands: "
        for i in $(seq $node_count); do
            echo "	l$i-cli, l$i-log, l$i-follow"
        done
    }

    connect_nodes() {
        L2_PORT=$(l2-cli getinfo | jq .binding[0].port)
        
        L2_PUBKEY=$(l2-cli getinfo | jq -r .id)

        L2_CONNECT="$L2_PUBKEY@127.0.0.1:$L2_PORT"
        
        echo $(l1-cli connect $L2_CONNECT)
    }
    rebuild() {
        make build
    	make copy
    	restart
    }
    restart() {
        stop_nigiri_env
        start_nigiri_env
    }
    restart_plugins() {
        l1-cli
    }

    setup_channel() {
        L1_ADDR=$(l1-cli newaddr)
        echo $(bt-cli generatetoaddress 12  $L1_ADDR)
        echo $(l1-cli fundchannel $L2_PUBKEY 10000000)
        bt-cli generatetoaddress 12  $L1_ADDR
    }

    fund_nodes() {
        l1_liquid=$(l1-cli liquid-wallet-getaddress)
        l1_liquid=$(sed -e 's/^"//' -e 's/"$//' <<<"$l1_liquid")
        echo $l1_liquid
        faucet $l1_liquid
    }

    faucet() {
    echo $1
        curl --header "Content-Type: application/json" \
          --request POST \
          --data '{"address":"$1"}' \
          http://localhost:3001/faucet
    }
     '';
}