from pyln.testing.fixtures import *
from pyln.client import Millisatoshi
import random
import string
import time


def test_liquid_swap_in(node_factory):
    swapAmt = 100000
    l1 = node_factory.get_node(options=getpluginOpts(get_random_string(8), "18884"))
    l2 = node_factory.get_node(options=getpluginOpts(get_random_string(8), "18885"))
    l1.daemon.wait_for_log(r"peerswap initialized")

    l1.connect(l2)
    l1.fundchannel(l2)

    scid12 = l1.get_channel_scid(l2)

    c12 = l2.rpc.listpeers(l1.info['id'])['peers'][0]['channels'][0]
    startingMsats = Millisatoshi(c12['to_us_msat'])

    l2.rpc.call("dev-liquid-faucet")
    time.sleep(1)
    l2Balance = l2.rpc.call("liquid-wallet-getbalance")
    assert l2Balance == 100000000

    l2.rpc.call("swap-in", {'amt':swapAmt,'short_channel_id':scid12})

    l2.daemon.wait_for_log(r".*Event_SwapInSender_OnTxMsgSent .*")
    time.sleep(1)
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    time.sleep(1)
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    time.sleep(1)
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l2.daemon.wait_for_log(r".*Event_OnClaimedPreimage")
    time.sleep(1)
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    time.sleep(1)


    l2Balance = l2.rpc.call("liquid-wallet-getbalance")

    # todo fix assertion with swap fee amount
    assert l2Balance <= 100000000 - swapAmt


    c12 = l2.rpc.listpeers(l1.info['id'])['peers'][0]['channels'][0]


    # todo fix assertion with swap fee amount
    assert Millisatoshi(c12['to_us_msat']) >= startingMsats + ((swapAmt-500) * 1000)


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