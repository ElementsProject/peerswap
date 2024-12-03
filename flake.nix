{
  inputs = {
    # Pinning to revision 6915a163f351c32bd4557518d047725665e83d37 
    # - cln v24.08.02
    # - lnd v0.18.3-beta
    # - bitcoin v28.0
    # - elements v23.2.1
    nixpkgs.url = "github:NixOS/nixpkgs/6915a163f351c32bd4557518d047725665e83d37";
    flake-utils.url = "github:numtide/flake-utils";
    # blockstream-electrs: init at 0.4.1 #299761
    # https://github.com/NixOS/nixpkgs/pull/299761/commits/680d27ad847801af781e0a99e4b87ed73965c69a
    nixpkgs2.url = "github:NixOS/nixpkgs/680d27ad847801af781e0a99e4b87ed73965c69a";
    # lwk: init at 9ddd20a806625bb40cd063ad61d80d106809a9fd
    # https://github.com/Blockstream/lwk/commit/9ddd20a806625bb40cd063ad61d80d106809a9fd
    lwk-flake = {
      url = "github:blockstream/lwk/9ddd20a806625bb40cd063ad61d80d106809a9fd";
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
              go_1_23
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
