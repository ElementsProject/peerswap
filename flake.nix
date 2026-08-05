{
  inputs = {
    # Base packages for devShell.
    # nixpkgs is pinned via `flake.lock` (currently rev 3e41b24abd260e8f71dbe2f5737d24122f972158).
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
    };

    lwk-flake = {
      url = "github:blockstream/lwk/1095b82575fdddc38534ea266d38eee5332350bf";
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
        # Version check (inside `nix develop`): `lwk_cli --version`
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
            # - `bitcoind --version` (bitcoind 31.0)
            # - `elementsd --version` (elementsd 23.3.3)
            bitcoind
            elementsd

            # Lightning implementations (used by integration tests / local setups).
            # Version checks:
            # - `lightningd --version` (Core Lightning / clightning 26.04.1)
            # - `lnd --version` (lnd 0.21.0-beta)
            clightning
            lnd

            # Wallet / Liquid tooling (lwk_cli 0.18.1).
            lwk
          ];

          hardeningDisable = [ "all" ];
        };
      }
    );
}
