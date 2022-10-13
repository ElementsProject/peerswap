let
# Pinning to revision 93dc85b485638bda9ac76458dcd45b997cc7cdbd 
# - cln v0.12.1 
# - lnd v0.15.2-beta
# - bitcoin v23.0
rev = "93dc85b485638bda9ac76458dcd45b997cc7cdbd";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

# Need an lnd v0.15.2-beta for an emergency fix in lnd.
lndemergencyrev = "d74b3668e7bf0eb2e765fdfcb6665b6d4b751edf";
lndemergencynixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${lndemergencyrev}.tar.gz";
lndemergencypkgs = import lndemergencynixpkgs {};

# Override priority for bitcoin as /bin/bitcoin_test will
# confilict with /bin/bitcoin_test from elementsd.
bitcoin = (pkgs.bitcoin.overrideAttrs (attrs: {
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
        lnd = lndemergencypkgs.lnd;
    };
    testpkgs = [ go bitcoin elementsd clightning-dev lndemergencypkgs.lnd ];
    devpkgs = [ bitcoin elementsd clightning clightning-dev lndemergencypkgs.lnd docker-compose jq nodePackages.mermaid-cli ];
}