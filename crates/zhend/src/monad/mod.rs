//! # Monad Bridge — Protocol Integration
//!
//! Zhen sits beneath Monad but communicates through it.
//! This module handles:
//! - Parsing IPv6 HbH extension headers to extract Zhen options
//! - Piggybacking Pu fragments onto outgoing Monad packets (Qi transport)
//! - Constructing context vectors from Monad flow metadata (De input)

pub mod hbh;
pub mod bridge;
