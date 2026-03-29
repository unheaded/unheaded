# Nix derivation for zhend.
# Build: nix-build nix/default.nix
# Or in a flake: inputs.zhend.packages.${system}.default

{ lib
, rustPlatform
, pkg-config
, protobuf
, openssl
}:

rustPlatform.buildRustPackage rec {
  pname = "zhend";
  version = "0.1.0";

  src = ./..;

  cargoLock = {
    lockFile = ../Cargo.lock;
    # If using git dependencies, add outputHashes here.
  };

  nativeBuildInputs = [
    pkg-config
    protobuf  # protoc for tonic-build
  ];

  buildInputs = [
    openssl  # for rustls ring backend
  ];

  # Enable PQ features by default.
  buildFeatures = [ "pq" ];

  # Run tests during build.
  doCheck = true;
  checkFlags = [
    # Skip tests that require network.
    "--skip" "integration"
  ];

  meta = with lib; {
    description = "Zhen Layer 0 — Anti-fragile knowledge substrate for the Unheaded Protocol";
    homepage = "https://github.com/unheaded/zhend";
    license = licenses.gpl3Only;
    maintainers = []; # TODO: add maintainer
    platforms = platforms.linux;
    mainProgram = "zhend";
  };
}
