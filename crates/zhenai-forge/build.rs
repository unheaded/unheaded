// build.rs — Tell the linker where to find ROCm/HIP libraries
fn main() {
    // ROCm HIP library path
    println!("cargo:rustc-link-search=native=/opt/rocm/lib");
    println!("cargo:rustc-link-lib=dylib=amdhip64");
    println!("cargo:rustc-link-lib=dylib=hipblas");
}
