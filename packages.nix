let
# Pinning to version 21.05 unstable to get elementsd 0.21.0.
rev = "a90064443bc2aaed44d02aa170484d317cb130de";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

nix-bitcoin-release = pkgs.fetchFromGitHub {
    owner = "fort-nix";
    repo = "nix-bitcoin";
    rev = "v0.0.59";
    sha256 = "sha256:1ms35sig935qw9i79i4g63p32h0qx27wvpr4rimi3wqrj92kfnjf";
};

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
clightning-dev = (nix-bitcoin-unstable-pkgs.clightning.overrideDerivation (attrs: {
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
        lnd = nix-bitcoin-unstable-pkgs.lnd;
    };
    testpkgs = [ go bitcoin elementsd clightning-dev nix-bitcoin-unstable-pkgs.lnd];
    devpkgs = [ bitcoin elementsd nix-bitcoin-unstable-pkgs.clightning clightning-dev nix-bitcoin-unstable-pkgs.lnd docker-compose jq nodePackages.mermaid-cli ];
}