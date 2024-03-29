let
# Pinning to revision f54322490f509985fa8be4ac9304f368bd8ab924 
# - cln v24.02.1
# - lnd v0.17.4-beta
# - bitcoin v26.0
# - elements v23.2.1

rev = "f54322490f509985fa8be4ac9304f368bd8ab924";
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
    devpkgs = [ go_1_22 gotools bitcoind elementsd clightning lnd ];

}
