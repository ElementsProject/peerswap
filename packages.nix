let
# Pinning explicitly to 21.05.
rev = "7e9b0dff974c89e070da1ad85713ff3c20b0ca97";
nixpkgs = fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
pkgs = import nixpkgs {};

nix-bitcoin-release = pkgs.fetchFromGitHub {
    owner = "fort-nix";
    repo = "nix-bitcoin";
    rev = "v0.0.49";
    sha256 = "03b1zpwbcz98vrcnfflnxdcg17mhip3p63asp3jqzzzk8v4f1rna";
};

nix-bitcoin-unstable-pkgs = import (import "${toString nix-bitcoin-release}/pkgs/nixpkgs-pinned.nix").nixpkgs-unstable {};

# Override priority for bitcoin as /bin/bitcoin_test will 
# confilict with /bin/bitcoin_test from elementsd.
bitcoin = (pkgs.bitcoin.overrideAttrs (attrs: {
    meta = attrs.meta or {} // {
        priority = 0;
    };
}));

# Override python with packages from ./python-packages
packageOverrides = pkgs.callPackage ./python-packages.nix { };
python = pkgs.python36.override { inherit packageOverrides; };
pythonWithPackages-dev = python.withPackages(ps: [ ps.pytest ps.pyln-client ps.pyln-testing ps.black ]);
pythonWithPackages-test = python.withPackages(ps: [ ps.pytest ps.pyln-client ps.pyln-testing ]);
in with pkgs;
{
    execs = {
        clightning = nix-bitcoin-unstable-pkgs.clightning;
        bitcoin = bitcoin;
        elements = nix-bitcoin-unstable-pkgs.elementsd;
    };
    testpkgs = [ go bitcoin nix-bitcoin-unstable-pkgs.elementsd nix-bitcoin-unstable-pkgs.clightning pythonWithPackages-test ];
    devpkgs = [ go bitcoin nix-bitcoin-unstable-pkgs.elementsd nix-bitcoin-unstable-pkgs.clightning docker-compose jq pythonWithPackages-dev ];
}