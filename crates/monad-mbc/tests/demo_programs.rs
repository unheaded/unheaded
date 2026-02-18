use monad_mbc::{assemble, disasm_listing};

// Gradient demo: fills 320x200 screen with diagonal color gradient.
// All instructions use the actual two-operand MBC format.
const GRADIENT_SRC: &str = r#"
    ; gradient.mbc - Fills screen with color gradient
    ; r1  = screen base (0xC000)
    ; r2  = y (0..199)
    ; r3  = height (200)
    ; r4  = x (0..319)
    ; r5  = width (320)
    ; r6  = pixel address
    ; r7  = row offset
    ; r8  = color
    ; r9  = 320 constant
    ; r13 = temp
    ; r14 = constant 1

    MOVI r1, 0xC000
    MOVI r3, 200
    MOVI r5, 320
    MOVI r9, 320
    MOVI r14, 1
    MOVI r2, 0

y_loop:
    MOVI r4, 0
    MOV r7, r2
    MUL r7, r9

x_loop:
    ; addr = screen_base + y*320 + x
    MOV r6, r7
    ADD r6, r4
    ADD r6, r1

    ; color = (x + y) & 0xFF
    MOV r8, r4
    ADD r8, r2

    ; store pixel
    STB [r6+0], r8

    ; x++
    ADD r4, r14
    CMP r4, r5
    JN x_loop

    ; y++
    ADD r2, r14
    CMP r2, r3
    JN y_loop

    ; draw frame: SYS_DRAW_FRAME (r0=1, r1=fb_ptr)
    MOVI r0, 1
    MOVI r1, 0xC000
    SYSCALL
    HALT
"#;

// Checkerboard demo: 8x8 tile pattern on 320x200 screen.
const CHECKERBOARD_SRC: &str = r#"
    ; checkerboard.mbc - 8x8 checkerboard pattern
    ; r1  = screen base
    ; r2  = y
    ; r3  = height (200)
    ; r4  = x
    ; r5  = width (320)
    ; r6  = pixel address
    ; r7  = row offset
    ; r8  = tile_x (x >> 3)
    ; r9  = tile_y (y >> 3)
    ; r10 = color
    ; r11 = 320 constant
    ; r12 = temp
    ; r13 = 1 constant
    ; r14 = 14 constant

    MOVI r1, 0xC000
    MOVI r3, 200
    MOVI r5, 320
    MOVI r11, 320
    MOVI r13, 1
    MOVI r14, 14
    MOVI r2, 0

y_loop:
    MOVI r4, 0
    MOV r7, r2
    MUL r7, r11
    MOV r9, r2
    SHR r9, 3

x_loop:
    ; pixel address
    MOV r6, r7
    ADD r6, r4
    ADD r6, r1

    ; tile_x = x >> 3
    MOV r8, r4
    SHR r8, 3

    ; checker = (tile_x + tile_y) & 1
    MOV r10, r8
    ADD r10, r9
    AND r10, r13

    ; color = checker * 14 + 1
    MUL r10, r14
    ADD r10, r13

    ; store pixel
    STB [r6+0], r10

    ; x++
    ADD r4, r13
    CMP r4, r5
    JN x_loop

    ; y++
    ADD r2, r13
    CMP r2, r3
    JN y_loop

    ; draw frame
    MOVI r0, 1
    MOVI r1, 0xC000
    SYSCALL
    HALT
"#;

#[test]
fn test_assemble_gradient() {
    let code = assemble(GRADIENT_SRC).expect("gradient should assemble");
    assert!(!code.is_empty(), "gradient should produce instructions");

    println!("Gradient: {} instructions", code.len());

    let listing = disasm_listing(&code);
    println!("=== Gradient Listing ===\n{}", listing);
    assert!(listing.contains("MOVI"), "listing should contain MOVI");
    assert!(listing.contains("STB"), "listing should contain STB");
    assert!(listing.contains("SYSCALL"), "listing should contain SYSCALL");
    assert!(listing.contains("HALT"), "listing should contain HALT");
}

#[test]
fn test_assemble_checkerboard() {
    let code = assemble(CHECKERBOARD_SRC).expect("checkerboard should assemble");
    assert!(!code.is_empty(), "checkerboard should produce instructions");

    println!("Checkerboard: {} instructions", code.len());

    let listing = disasm_listing(&code);
    println!("=== Checkerboard Listing ===\n{}", listing);
    assert!(listing.contains("SHR"), "listing should contain SHR");
    assert!(listing.contains("MUL"), "listing should contain MUL");
    assert!(listing.contains("AND"), "listing should contain AND");
}
