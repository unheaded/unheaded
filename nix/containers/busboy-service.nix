{ config, pkgs, ... }:

{
  # =============================================================================
  # BUSBOY SERVICE CONTAINER (Legacy Compatibility)
  # =============================================================================
  # Alias for busboy.nix - maintains backward compatibility.
  #
  # Service: Message bus (gRPC + REST)
  # IP: 10.10.10.11
  # Note: This is a duplicate for service discovery patterns
  # =============================================================================

  imports = [
    ./busboy.nix
  ];

  # Override IP for this instance if needed
  unheaded.networking.serviceIP = "10.10.10.11";
}
