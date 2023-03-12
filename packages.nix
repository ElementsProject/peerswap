let
# Pinning to revision 5ae751c41b1b78090e4c311f43aa34792599e563 
# - cln v23.02
# - lnd v0.15.5-beta
# - bitcoin v24.0.1
# - elements v22.1.0

rev = "5ae751c41b1b78090e4c311f43aa34792599e563";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

# bitcoin v24.0.1 installation breaks on darwin. Temporarily revert to v24.0
bitcoin-fix-rev = "0a8826e7582cf357c1248e10653fd946e6570f99";
bitcoin-fix-nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${bitcoin-fix-rev}.tar.gz";
bitcoin-fix-pkgs = import bitcoin-fix-nixpkgs {};

# Override priority for bitcoin as /bin/bitcoin_test will
# confilict with /bin/bitcoin_test from elementsd.
bitcoin = (bitcoin-fix-pkgs.bitcoin.overrideAttrs (attrs: {
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
        bitcoin = bitcoin;
        elements = elementsd;
        mermaid = nodePackages.mermaid-cli;
        lnd = lnd;
    };
    testpkgs = [ go bitcoin elementsd clightning-dev lnd ];
    devpkgs = [ go bitcoin elementsd clightning clightning-dev lnd docker-compose jq nodePackages.mermaid-cli ];
}