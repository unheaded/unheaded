
; Call/Return test - verify stack operations work correctly
; Expected: r0 = 42, r2 = 99
    MOVI r0, 0
    CALL get_42
    MOVI r2, 99
    HALT

get_42:
    MOVI r0, 42
    RET
