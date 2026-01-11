{
  inputs = {
    # Base packages for devShell.
    # nixpkgs is pinned via `flake.lock` (currently rev f997fa0f94fb1ce55bccb97f60d41412ae8fde4c).
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs = {
        nixpkgs.follows = "nixpkgs";
      };
    };
    crane = {
      url = "github:ipetkov/crane";
    };
    blockstream-electrs = {
      url = "github:Blockstream/electrs";
      inputs = {
        nixpkgs.follows = "nixpkgs";
        flake-utils.follows = "flake-utils";
        rust-overlay.follows = "rust-overlay";
        crane.follows = "crane";
      };
    };

    lwk-flake = {
      url = "github:blockstream/lwk/9ddd20a806625bb40cd063ad61d80d106809a9fd";
      inputs = {
        nixpkgs.follows = "nixpkgs";
        flake-utils.follows = "flake-utils";
        crane.follows = "crane";
        rust-overlay.follows = "rust-overlay";
      };
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      lwk-flake,
      blockstream-electrs,
      ...
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          system = system;
        };

        # Liquid-capable electrs build (from the Blockstream electrs flake input).
        # Version check (inside `nix develop`): `electrs --version`
        electrs-liquid = blockstream-electrs.packages.${system}.binLiquid;

        # Liquid Wallet Kit CLI (pinned to the commit in `inputs.lwk-flake.url`).
        # Version check (inside `nix develop`): `lwk --version`
        lwk = lwk-flake.packages.${system}.bin;
      in
      with pkgs;
      {
        devShells.default = mkShell {
          buildInputs = [
            # Go toolchain (go 1.25.4; build/test). Version check: `go version`
            go
            # Go tooling bundle (gotools 0.34.0; e.g. `gopls`, `goimports`). Version checks:
            # - `gopls version`
            # - `goimports -h` (prints usage; some builds don't support `--version`)
            gotools

            # Liquid indexer (electrs 0.4.1; used by dev/test flows that need Liquid chain data).
            electrs-liquid

            # Bitcoin/Elements daemons (used by integration tests / local networks).
            # Version checks:
            # - `bitcoind --version` (bitcoind 30.0)
            # - `elementsd --version` (elementsd 23.2.4)
            bitcoind
            elementsd

            # Lightning implementations (used by integration tests / local setups).
            # Version checks:
            # - `lightningd --version` (Core Lightning / clightning 25.09.2)
            # - `lnd --version` (lnd 0.19.3-beta)
            clightning
            lnd

            # Wallet / Liquid tooling (lwk_cli 0.8.0).
            lwk
          ];

          hardeningDisable = [ "all" ];
        };
      }
    );
}
