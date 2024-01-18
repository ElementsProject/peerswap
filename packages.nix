let
# Pinning to revision c8a66e2bb84c2b9cf7014a61c44ffb13ef4317ed
# - cln v23.11
# - lnd v0.17.0-beta
# - bitcoin v25.1
# - elements v23.2.1

rev = "c8a66e2bb84c2b9cf7014a61c44ffb13ef4317ed";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

# Override priority for bitcoin as /bin/bitcoin_test will
# confilict with /bin/bitcoin_test from elementsd.
bitcoind = (pkgs.bitcoind.overrideAttrs (attrs: {
    meta = attrs.meta or {} // {
        priority = 0;
    };
}));

in with pkgs;
{
    execs = {
        clightning = clightning;
        bitcoind = bitcoind;
        elementsd = elementsd;
        mermaid = nodePackages.mermaid-cli;
        lnd = lnd;
    };
    testpkgs = [ go bitcoind elementsd lnd ];
    devpkgs = [ go_1_19 gotools bitcoind elementsd clightning lnd ];
}
