//! Traffic pattern analysis → knowledge geography.
//!
//! Observes which fragments are accessed together, building an implicit
//! co-access graph. Over time, clusters emerge — Li reveals itself.

// TODO: co-access adjacency matrix (sparse, fragment_id × fragment_id)
// TODO: sliding window of recent access pairs
// TODO: community detection (Louvain or label propagation)
// TODO: export topology snapshot for visualization (Cloak dashboard)
