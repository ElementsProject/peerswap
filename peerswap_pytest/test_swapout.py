from pyln.testing.fixtures import *
from pyln.client import Millisatoshi
import random
import string


# def test_your_plugin(node_factory, bitcoind):
#     l1 = node_factory.get_node(options=pluginopt)
#     s = l1.rpc.getinfo()
#     assert(s['network'] == 'regtest') # or whatever you want to test

# def test_liquid_address(node_factory, bitcoind):
#     l1 = node_factory.get_node(options=getpluginOpts("bvdspdofgjsdfg"))
#     addr = l1.rpc.call(method="liquid-wallet-getaddress")
#     #l1.daemon.wait_for_log(r"\[Wallet\] Getting address .*")
#     assert l1.daemon.is_in_log(r"\[Wallet\] Getting address .*")
# def test_generate(node_factory, bitcoind):
#     l1 = node_factory.get_node(options=getpluginOpts(get_random_string(8)))
#     l1.daemon.wait_for_log(r"peerswap initialized")
#     addr = l1.rpc.call(method="liquid-wallet-getaddress")
#     res = l1.rpc.call("dev-liquid-generate", {'amount':"2"})
#    print(res)
def test_liquid_swap_in(node_factory):
    swapAmt = 100000
    l1 = node_factory.get_node(options=getpluginOpts(get_random_string(8)))
    l2 = node_factory.get_node(options=getpluginOpts(get_random_string(8)))
    l1.daemon.wait_for_log(r"peerswap initialized")

    l1.connect(l2)
    l1.fundchannel(l2)

    scid12 = l1.get_channel_scid(l2)

    c12 = l2.rpc.listpeers(l1.info['id'])['peers'][0]['channels'][0]
    startingMsats = Millisatoshi(c12['to_us_msat'])

    l2.rpc.call("dev-liquid-faucet")

    l2Balance = l2.rpc.call("liquid-wallet-getbalance")
    assert l2Balance == 100000000

    l2.rpc.call("swap-in", {'amt':swapAmt,'short_channel_id':scid12})

    l2.daemon.wait_for_log(r".*Event_SwapInSender_OnTxMsgSent .*")

    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l2.daemon.wait_for_log(r".*Event_OnClaimedPreimage")

    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l1.rpc.call("dev-liquid-generate", {'amount':1})

    l2Balance = l2.rpc.call("liquid-wallet-getbalance")

    # todo fix assertion with swap fee amount
    assert l2Balance <= 100000000 - swapAmt


    c12 = l2.rpc.listpeers(l1.info['id'])['peers'][0]['channels'][0]


    # todo fix assertion with swap fee amount
    assert Millisatoshi(c12['to_us_msat']) >= startingMsats + ((swapAmt-500) * 1000)
def test_liquid_swap_out(node_factory, bitcoind):
    swapAmt = 100000
    l1 = node_factory.get_node(options=getpluginOpts(get_random_string(8)))
    l2 = node_factory.get_node(options=getpluginOpts(get_random_string(8)))
    l1.daemon.wait_for_log(r"peerswap initialized")

    l1.connect(l2)
    l1.fundchannel(l2)

    scid12 = l1.get_channel_scid(l2)

    c12 = l2.rpc.listpeers(l1.info['id'])['peers'][0]['channels'][0]
    startingMsats = Millisatoshi(c12['to_us_msat'])

    l2.rpc.call("dev-liquid-faucet")

    l2Balance = l2.rpc.call("liquid-wallet-getbalance")
    assert l2Balance == 100000000

    l1.rpc.call("swap-out", {'amt':swapAmt,'short_channel_id':scid12})

    l2.daemon.wait_for_log(r".*Event_SwapOutReceiver_TxBroadcasted .*")

    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l1.daemon.wait_for_log(r".*Event_SwapOutSender_FinishSwap")

    l1.rpc.call("dev-liquid-generate", {'amount':1})
    l1.rpc.call("dev-liquid-generate", {'amount':1})

    l2Balance = l2.rpc.call("liquid-wallet-getbalance")

    # todo fix assertion with swap fee amount
    assert l2Balance <= 100000000 - swapAmt


    c12 = l2.rpc.listpeers(l1.info['id'])['peers'][0]['channels'][0]


    # todo fix assertion with swap fee amount
    assert Millisatoshi(c12['to_us_msat']) >= startingMsats + ((swapAmt-500) * 1000)


def getpluginOpts(walletname):
    return {
        'plugin': os.path.join(os.path.dirname(__file__), "../peerswap"),
        'peerswap-liquid-rpchost': 'http://localhost',
        'peerswap-liquid-rpcport': '18884',
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
# --peerswap-liquid-rpchost=http://localhost \
# --peerswap-liquid-rpcport=7041 \
# --peerswap-liquid-rpcuser=admin1 \
# --peerswap-liquid-rpcpassword=123 \
# --peerswap-liquid-network=regtest \
# --peerswap-liquid-rpcwallet=swap-$i