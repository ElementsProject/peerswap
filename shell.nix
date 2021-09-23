let
    peerswap-pkgs = import ./packages.nix;
in
{ pkgs ? (import <nixpkgs> {})}:
let
    execs = peerswap-pkgs.execs;
in with pkgs;
stdenv.mkDerivation rec {
    name = "peerswap-dev-env";
    # nativeBuildInputs = [nodejs];
    buildInputs = [openssl peerswap-pkgs.devpkgs];
    
    shellHook = ''
    alias lightning-cli='${execs.clightning}/bin/lightning-cli'
    alias lightningd='${execs.clightning}/bin/lightningd'
    alias bitcoind='${execs.bitcoin}/bin/bitcoind'
    alias bitcoin-cli='${execs.bitcoin}/bin/bitcoin-cli'
    alias elementsd='${execs.elements}/bin/elementsd'
    alias elements-cli='${execs.elements}/bin/elements-cli'

    . ./contrib/startup_regtest.sh
    setup_alias
    '';
}
