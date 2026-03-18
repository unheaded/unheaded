# Unheaded Protocol Extension Mechanisms

The Unheaded protocol supports extensions through two primary mechanisms: TLV (Type-Length-Value) containers within the Monad Hop-by-Hop extension header, and the Sophia dictionary's hierarchical entry system.

## TLV Extension Container

The Monad IPv6 Hop-by-Hop header includes a TLV container (defined in foundation spec draft-05, Section M6) that allows new metadata fields to be carried alongside the 20-byte Monad register. Each TLV entry consists of a 1-byte type, 1-byte length, and variable-length value. Unrecognized TLV types are skipped by conforming implementations, ensuring backward compatibility.

## Sophia Dictionary Extensions

New entry types can be added to the Sophia dictionary system without protocol changes. Each entry type is identified by a sub-dictionary index and follows the standard CBOR serialization format. Custom entry types (routing, firewall, IDS, health, observability) were added in sophia-dictionary draft-02 using this mechanism.

## Ring Path Counter (M8)

The Ring Path Counter extension (foundation spec M8) adds per-hop ring buffer usage tracking via the TLV container. This enables operators to monitor ring buffer saturation across the packet path without additional out-of-band signaling.

## Extension Registry

Extension types are tracked in the IANA registries defined by the foundation specification (draft-05, Section 11). New extensions require Standards Action or Specification Required registration, depending on the registry.

## Adding Extensions

To propose a new extension: define the wire format in a TLV entry, implement the eBPF processing hook, register the type code, and add a Sophia dictionary entry if the extension carries stateful metadata.
