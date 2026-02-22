; Fibonacci sequence — compute fib(10) in r0
; Expected result: r0 = 55
    MOVI r0, 0       ; fib(0) = 0
    MOVI r1, 1       ; fib(1) = 1
    MOVI r2, 10      ; counter
loop:
    MOV r3, r1       ; temp = b
    ADD r1, r0       ; b = a + b
    MOV r0, r3       ; a = temp
    ADDI r2, -1      ; counter--
    JNZ loop         ; if counter != 0, loop
    HALT
