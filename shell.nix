# Just uses the flake. For the nix-env addon users.
(builtins.getFlake ("git+file://" + toString ./.)).devShells.${builtins.currentSystem}.default
