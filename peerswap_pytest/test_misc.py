from pyln.testing.fixtures import *
from pyln.client import Millisatoshi
import random
import string
import time

# todo fix test
# def test_your_plugin(node_factory, bitcoind):
#     l1 = node_factory.get_node(options=getpluginOpts(get_random_string(8), "18884"))
#     l2 = node_factory.get_node(options=getpluginOpts(get_random_string(8), "18885"))
#
#     l1.connect(l2)
#     l2.daemon.wait_for_log(r'{}-.*-chan#[0-9]*: Handed peer, entering loop'.format(l1.info['id']))
#     bitcoind.generate_block(5)
#     l2getinfo = l2.rpc.getinfo()
#     time.sleep(5)
#     bitcoind.generate_block(5)
#     time.sleep(5)
#     res = l1.rpc.call("peerswap-peers")
#     print(l1.rpc.listnodes())
#     print(l2getinfo)
#     print(res)
#     time.sleep(3)

def test_sendtoaddres(node_factory):
    l1 = node_factory.get_node(options=getpluginOpts(get_random_string(8), "18884"))
    l2 = node_factory.get_node(options=getpluginOpts(get_random_string(8), "18885"))
    l1.daemon.wait_for_log(r"peerswap initialized")
    l2.daemon.wait_for_log(r"peerswap initialized")

    l2.rpc.call("dev-liquid-faucet")
    time.sleep(1)
    l2Balance = l2.rpc.call("peerswap-liquid-getbalance")
    assert l2Balance == 100000000
    time.sleep(1)
    addr = l1.rpc.call("peerswap-liquid-getaddress")
    l2.rpc.call("peerswap-liquid-sendtoaddress",{'address':addr,'amount_sat':1000})
    l1.rpc.call("dev-liquid-generate")
    time.sleep(1)
    l1.rpc.call("dev-liquid-generate")
    time.sleep(1)
    l1.rpc.call("dev-liquid-generate")
    time.sleep(3)


    l1Balance = l1.rpc.call("peerswap-liquid-getbalance")
    assert l1Balance == 1000


def getpluginOpts(walletname, rpcport):
    return {
        'plugin': os.path.join(os.path.dirname(__file__), "../peerswap"),
        'peerswap-liquid-rpchost': 'http://localhost',
        'peerswap-liquid-rpcport': rpcport,
        'peerswap-liquid-rpcuser': 'admin1',
        'peerswap-liquid-rpcpassword': 123,
        'peerswap-liquid-network': 'regtest',
        'peerswap-liquid-rpcwallet': walletname
    }

def get_random_string(length):
    # choose from all lowercase letter
    letters = string.ascii_lowercase
    result_str = ''.join(random.choice(letters) for i in range(length))
    return result_str

def only_one(arr):
    """Many JSON RPC calls return an array; often we only expect a single entry
    """
    assert len(arr) == 1
    return arr[0]

def wait_for(success, timeout=10):
    start_time = time.time()
    interval = 0.25
    while not success():
        time_left = start_time + timeout - time.time()
        if time_left <= 0:
            raise ValueError("Timeout while waiting for {}", success)
        time.sleep(min(interval, time_left))
        interval *= 2
        if interval > 5:
            interval = 5