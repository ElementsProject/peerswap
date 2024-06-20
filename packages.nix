let
    fetchNixpkgs = rev: fetchTarball "https://github.com/NixOS/nixpkgs/archive/${rev}.tar.gz";
    # Pinning to revision 755b915a158c9d588f08e9b08da9f7f3422070cc 
    # - cln v24.05
    # - lnd v0.18.0-beta
    # - bitcoin v27.1
    # - elements v23.2.1
    rev1 = "755b915a158c9d588f08e9b08da9f7f3422070cc";
    nixpkgs1 = fetchNixpkgs rev1;
    pkgs1 = import nixpkgs1 {};

    # Override priority for bitcoin as /bin/bitcoin_test will
    # confilict with /bin/bitcoin_test from elementsd.
    bitcoind = (pkgs1.bitcoind.overrideAttrs (attrs: {
        meta = attrs.meta or {} // {
            priority = 0;
        };
    }));
    # lwk: init at 0.3.0 #292522
    # https://github.com/NixOS/nixpkgs/pull/292522/commits/2b3750792b2e4b52f472b6e6d88a6b02b6536c43
    rev2 = "2b3750792b2e4b52f472b6e6d88a6b02b6536c43";
    nixpkgs2 = fetchNixpkgs rev2;
    pkgs2 = import nixpkgs2 {};
    # blockstream-electrs: init at 0.4.1 #299761
    # https://github.com/NixOS/nixpkgs/pull/299761/commits/680d27ad847801af781e0a99e4b87ed73965c69a
    rev3 = "680d27ad847801af781e0a99e4b87ed73965c69a";
    nixpkgs3 = fetchNixpkgs rev3;
    pkgs3 = import nixpkgs3 {};
    blockstream-electrs = pkgs3.blockstream-electrs.overrideAttrs (oldAttrs: {
        cargoBuildFlags = [ "--features liquid" "--bin electrs" ];
    });

in
{
    execs = {
        clightning = pkgs1.clightning;
        bitcoind = bitcoind;
        elementsd = pkgs1.elementsd;
        mermaid = pkgs1.nodePackages.mermaid-cli;
        lnd = pkgs1.lnd;
        lwk = pkgs2.lwk;
        electrs = blockstream-electrs;

    };
    testpkgs = [ pkgs1.go pkgs1.bitcoind pkgs1.elementsd pkgs1.lnd pkgs2.lwk blockstream-electrs];
    devpkgs = [ pkgs1.go_1_22 pkgs1.gotools pkgs1.bitcoind pkgs1.elementsd pkgs1.clightning pkgs1.lnd pkgs2.lwk blockstream-electrs];
}
