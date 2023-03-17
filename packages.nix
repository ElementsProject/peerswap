let
# Pinning to revision e928e936fbf344e10a0979b574bf1823063d37e4 
# - cln v23.02.2
# - lnd v0.15.5-beta
# - bitcoin v24.0.1
# - elements v22.1.0

rev = "e928e936fbf344e10a0979b574bf1823063d37e4";
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
    devpkgs = [ bitcoind elementsd clightning clightning-dev lnd docker-compose jq nodePackages.mermaid-cli ];
}