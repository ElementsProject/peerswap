{ pkgs ? (import <nixpkgs> {})}:
with pkgs.lib;
let
  nix-bitcoin-release = pkgs.fetchFromGitHub {
      owner = "fort-nix";
      repo = "nix-bitcoin";
      rev = "v0.0.49";
      sha256 = "03b1zpwbcz98vrcnfflnxdcg17mhip3p63asp3jqzzzk8v4f1rna";
  };

  nix-bitcoin = pkgs.callPackage nix-bitcoin-release {};


  nixpkgs-unstable-path = (import "${toString nix-bitcoin-release}/pkgs/nixpkgs-pinned.nix").nixpkgs-unstable;
  nixpkgs-unstable = import nixpkgs-unstable-path { };

  packageOverrides = pkgs.callPackage ./python-packages.nix {};
  python = pkgs.python36.override { inherit packageOverrides; };
  my-python-packages = python-packages: with python-packages; [
    pip
  ];
  pythonWithPackages = python.withPackages(ps: [ ps.pytest ps.pyln-client ps.pyln-testing ps.python-bitcoinrpc ps.black]);

in
with pkgs;
    stdenv.mkDerivation rec {
     name = "peerswap-environment";
     # python packages python39Full python39Packages.pip python39Packages.bitcoinlib sqlite
     li = nixpkgs-unstable.clightning;
     nativeBuildInputs = [openssl ];
     buildInputs = [pythonWithPackages docker-compose bitcoin nixpkgs-unstable.elementsd li jq nixpkgs-unstable.go];
     path = lib.makeBinPath [  ];
     
     shellHook = ''

     alias lightning-cli='${li}/bin/lightning-cli'
     alias lightningd='${li}/bin/lightningd'
     alias bitcoind='${bitcoin}/bin/bitcoind'
     alias bitcoin-cli='${bitcoin}/bin/bitcoin-cli'

    . ./contrib/startup_regtest.sh
    # setup_pyln
    setup_alias
     '';

}