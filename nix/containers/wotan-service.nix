{ config, pkgs, ... }:

{
  # =============================================================================
  # WOTAN SERVICE CONTAINER (Legacy Compatibility)
  # =============================================================================
  # Alias for wotan.nix - maintains backward compatibility.
  #
  # Service: Message bus (gRPC + REST)
  # IP: 10.10.10.11
  # Note: This is a duplicate for service discovery patterns
  # =============================================================================

  imports = [
    ./wotan.nix
  ];

  # Override IP for this instance if needed
  unheaded.networking.serviceIP = "10.10.10.11";
}
