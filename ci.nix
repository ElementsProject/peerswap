let
    peerswap-pkgs = import ./packages.nix;
in with peerswap-pkgs; [
    testpkgs
]