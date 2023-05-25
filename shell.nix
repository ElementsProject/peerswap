let
    peerswap-pkgs = import ./packages.nix;
in
{ pkgs ? (import <nixpkgs> {})}:
let
    execs = peerswap-pkgs.execs;
in with pkgs;
stdenv.mkDerivation rec {
    name = "peerswap-dev-env";
    nativeBuildInputs = [openssl];
    buildInputs = [peerswap-pkgs.devpkgs ];
    
    shellHook = ''
    alias lightning-cli='${execs.clightning}/bin/lightning-cli'
    alias lightningd='${execs.clightning}/bin/lightningd'
    alias bitcoind='${execs.bitcoind}/bin/bitcoind'
    alias bitcoin-cli='${execs.bitcoind}/bin/bitcoin-cli'
    alias elementsd='${execs.elementsd}/bin/elementsd'
    alias elements-cli='${execs.elementsd}/bin/elements-cli'
    alias lightningd-dev='${execs.clightning-dev}/bin/lightningd-dev'
    alias lnd='${execs.lnd}/bin/lnd'
    alias lncli='${execs.lnd}/bin/lncli'

    . ./contrib/startup_regtest.sh
    setup_alias
    '';
}
