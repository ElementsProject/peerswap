{
  inputs = {
    # Pinning to revision fc13ad88bf7dbf2addb58f7e9d128c5547f59e93 
    # - cln v24.08
    # - lnd v0.18.2-beta
    # - bitcoin v27.1
    # - elements v23.2.1
    nixpkgs.url = "github:NixOS/nixpkgs/fc13ad88bf7dbf2addb58f7e9d128c5547f59e93";
    flake-utils.url = "github:numtide/flake-utils";
    # blockstream-electrs: init at 0.4.1 #299761
    # https://github.com/NixOS/nixpkgs/pull/299761/commits/680d27ad847801af781e0a99e4b87ed73965c69a
    nixpkgs2.url = "github:NixOS/nixpkgs/680d27ad847801af781e0a99e4b87ed73965c69a";
    # lwk: init at wasm_0.6.3 #14bac28
    # https://github.com/Blockstream/lwk/releases/tag/wasm_0.6.3
    lwk-flake = {
      url = "github:blockstream/lwk/14bac284fe712dd6fdbbbe82bda179a2a236b2fa";
      inputs = {
        nixpkgs.follows = "nixpkgs";
        flake-utils.follows = "flake-utils";
      };
    };
  };
  outputs = { self, nixpkgs, nixpkgs2, flake-utils, lwk-flake }:
    flake-utils.lib.eachDefaultSystem
      (system:
        let
          pkgs = import nixpkgs {
            system = system;
          };
          pkgs2 = import nixpkgs2 {
            system = system;
          };
          blockstream-electrs = pkgs2.blockstream-electrs.overrideAttrs (oldAttrs: {
            cargoBuildFlags = [ "--features liquid" "--bin electrs" ];
          });
          bitcoind = pkgs.bitcoind.overrideAttrs (attrs: {
            meta = attrs.meta or { } // { priority = 0; };
          });
          lwk = lwk-flake.packages.${system}.bin;
        in
        with pkgs;
        {
          devShells.default = mkShell {
            buildInputs = [
              go_1_22
              gotools
              blockstream-electrs
              bitcoind
              elementsd
              clightning
              lnd
              lwk
            ];
            # Cannot run the debugger without this
            # see https://github.com/go-delve/delve/issues/3085
            hardeningDisable = [ "all" ];
          };
        }
      );
}
