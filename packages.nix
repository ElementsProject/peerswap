let
# Pinning to revision be28f5521e1d3fec2bd22928f721120725aba272
# - cln v23.08 (with fix for clnrest.py crash on NixOS)
# - lnd v0.16.3-beta
# - bitcoin v25.0
# - elements v22.1.1

rev = "be28f5521e1d3fec2bd22928f721120725aba272";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

# Override priority for bitcoin as /bin/bitcoin_test will
# confilict with /bin/bitcoin_test from elementsd.
bitcoind = (pkgs.bitcoind.overrideAttrs (attrs: {
    meta = attrs.meta or {} // {
        priority = 0;
    };
}));

# Build a clightning version with developer features enabled.
# Clightning is way more responsive with dev features.
clightning-dev = (pkgs.clightning.overrideDerivation (attrs: {
    configureFlags = [ "--enable-developer" "--disable-valgrind" ];

    pname = "clightning-dev";
    postInstall = ''
        mv $out/bin/lightningd $out/bin/lightningd-dev
    '';
}));

in with pkgs;
{
    execs = {
        clightning = clightning;
        clightning-dev = clightning-dev;
        bitcoind = bitcoind;
        elementsd = elementsd;
        mermaid = nodePackages.mermaid-cli;
        lnd = lnd;
    };
    testpkgs = [ go bitcoind elementsd clightning-dev lnd ];
    devpkgs = [ go_1_19 gotools bitcoind elementsd clightning clightning-dev lnd ];
}