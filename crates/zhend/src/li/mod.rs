//! # Li (理) -- Emergent Pattern
//!
//! The pattern in jade. The grain in wood. The riverbed carved by water.
//! Li is not designed -- it is observed. Over time, traffic patterns
//! carve structure into Zhen's knowledge topology.
//!
//! Li modules observe but never impose.

pub mod strata;
pub mod topology;

pub use strata::{GeologicalTrend, StrataHistory, StrataSnapshot};
pub use topology::{TopologyEdge, TopologyNode, TopologySnapshot, TopologyTracker};
