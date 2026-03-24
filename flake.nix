{
  description = "kgateway development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
    rust-overlay = {
      url = "github:oxalica/rust-overlay";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, flake-utils, rust-overlay }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        overlays = [ (import rust-overlay) ];
        pkgs = import nixpkgs { inherit system overlays; };

        # Rust toolchain for internal/envoyinit/rustformations
        rustToolchain = pkgs.rust-bin.stable."1.86.0".default.override {
          extensions = [ "rustfmt" "clippy" ];
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Prompt
            starship

            # Go (pinned to match go.mod: go 1.26.x)
            go_1_26

            # Rust (for internal/envoyinit/rustformations)
            rustToolchain

            # Kubernetes tooling
            kubectl
            kind
            kubernetes-helm
            tilt

            # Protobuf
            protobuf
            buf

            # Code generation & build
            gnumake
            gcc
            pkg-config

            # CLI utilities
            yq-go
            jq
            curl
            wget
            git
            unzip

            # Container tooling
            docker-client

            # Linting GitHub Actions (also available via `go tool actionlint`)
            actionlint
          ];

          shellHook = ''
            export STARSHIP_CONFIG="$PWD/.starship.toml"
            if [ -n "$BASH_VERSION" ]; then
              eval "$(starship init bash 2>/dev/null)"
            elif [ -n "$ZSH_VERSION" ]; then
              eval "$(starship init zsh 2>/dev/null)"
            else
              case "$SHELL" in
                *zsh) eval "$(starship init zsh 2>/dev/null)" ;;
                *)    eval "$(starship init bash 2>/dev/null)" ;;
              esac
            fi

            echo "kgateway dev shell"
            echo "  Go:      $(go version)"
            echo "  Rust:    $(rustc --version)"
            echo "  kubectl: $(kubectl version --client --short 2>/dev/null || kubectl version --client)"
            echo "  Helm:    $(helm version --short)"
            echo "  Kind:    $(kind version)"
            echo ""
            echo "Run 'make help' to see available targets."

            # Most Go CLI tools (controller-gen, ginkgo, golangci-lint, etc.)
            # are managed via go.mod and invoked with 'go tool <name>'.
            # No need to install them separately.
          '';

          # Ensure go tool dependencies resolve correctly
          env = {
            GO111MODULE = "on";
            GOTOOLCHAIN = "local";
          };
        };
      });
}
