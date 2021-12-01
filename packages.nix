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

nix-bitcoin-unstable-pkgs = import (import "${toString nix-bitcoin-release}/pkgs/nixpkgs-pinned.nix").nixpkgs-unstable {};

# Override priority for bitcoin as /bin/bitcoin_test will 
# confilict with /bin/bitcoin_test from elementsd.
bitcoin = (pkgs.bitcoin.overrideAttrs (attrs: {
    meta = attrs.meta or {} // {
        priority = 0;
    };
}));

in with pkgs;
{
    execs = {
        clightning = nix-bitcoin-unstable-pkgs.clightning;
        bitcoin = bitcoin;
        elements = elementsd;
        mermaid = nodePackages.mermaid-cli;
    };
    testpkgs = [ go bitcoin elementsd nix-bitcoin-unstable-pkgs.clightning ];
    devpkgs = [ bitcoin elementsd nix-bitcoin-unstable-pkgs.clightning docker-compose jq nodePackages.mermaid-cli ];
}