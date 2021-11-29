let
# Pinning to version 21.05 unstable to get elementsd 0.21.0.
rev = "a90064443bc2aaed44d02aa170484d317cb130de";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

nix-bitcoin-release = pkgs.fetchFromGitHub {
    owner = "fort-nix";
    repo = "nix-bitcoin";
    rev = "v0.0.49";
    sha256 = "03b1zpwbcz98vrcnfflnxdcg17mhip3p63asp3jqzzzk8v4f1rna";
};

peerswap-lnd-pkgs = pkgs.fetchFromGitHub {
    owner = "sputn1ck";
    repo = "lnd";
    rev = "nix-build";
    sha256 = "sha256:12fm0gg5jn8ajsnzdm9xgp0gm8rh6fnscgqhiqnj96p4lgyn9yxk";
};

lndpkg = pkgs.callPackage peerswap-lnd-pkgs {};

nix-bitcoin-unstable-pkgs = import (import "${toString nix-bitcoin-release}/pkgs/nixpkgs-pinned.nix").nixpkgs-unstable {};

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
    configurePhase = "./configure --prefix=$out --enable-developer --disable-valgrind\n";
    pname = "clightning-dev";
    postInstall = ''
        mv $out/bin/lightningd $out/bin/lightningd-dev
    '';
}));

in with pkgs;
{
    execs = {
        clightning = nix-bitcoin-unstable-pkgs.clightning;
        clightning-dev = clightning-dev;
        bitcoin = bitcoin;
        elements = elementsd;
        mermaid = nodePackages.mermaid-cli;
        lnd = lndpkg;
    };
    testpkgs = [ go bitcoin elementsd clightning-dev ];
    devpkgs = [ bitcoin elementsd nix-bitcoin-unstable-pkgs.clightning clightning-dev docker-compose jq nodePackages.mermaid-cli ];
}