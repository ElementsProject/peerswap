let
# Pinning to revision e0361a947ed3eb692e43786f78e86075395ef3af for cln v0.11.1 
# and lnd v0.15.0-beta.
rev = "e0361a947ed3eb692e43786f78e86075395ef3af";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

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
    src = pkgs.fetchgit {
        url = "https://github.com/nepet/lightning.git";
        rev = "bf80565bc5f958be1d562b15ebd18586d8b0820e";
        sha256 = "sha256-rRX51vqCfUGxCSDt5Wptq0pfAl6tK2CIcNha4eiXtUM=";
    };
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
    testpkgs = [ go bitcoin elementsd clightning-dev lnd];
    devpkgs = [ bitcoin elementsd clightning clightning-dev lnd docker-compose jq nodePackages.mermaid-cli ];
}