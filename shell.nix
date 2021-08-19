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

 pkgs = import <nixpkgs> {};
in
with pkgs;
    stdenv.mkDerivation rec {
     name = "peerswap-environment";
     # python packages python39Full python39Packages.pip python39Packages.bitcoinlib sqlite
     li = nixpkgs-unstable.clightning;

     buildInputs = [docker-compose act bitcoin protoc-gen-go protoc-gen-go-grpc nixpkgs-unstable.elementsd li xonsh];
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