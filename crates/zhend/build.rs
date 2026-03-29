fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Compile proto files when they exist and protoc is available.
    // Gracefully skip if protoc not installed (CI will catch).
    if std::path::Path::new("proto/zhen.proto").exists() {
        match tonic_build::compile_protos("proto/zhen.proto") {
            Ok(_) => println!("cargo:warning=proto compiled successfully"),
            Err(e) => {
                println!("cargo:warning=proto compilation skipped: {e}");
                println!("cargo:warning=install protoc to enable gRPC codegen");
            }
        }
    }
    Ok(())
}
