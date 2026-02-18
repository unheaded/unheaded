fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_build::configure()
        .build_server(false) // We only need client
        .compile_protos(&["proto/topic.proto"], &["proto"])?;
    Ok(())
}
