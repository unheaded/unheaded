typedef unsigned int   uint;
typedef unsigned short ushort;
typedef unsigned char  uchar;

typedef unsigned char uint8;
typedef unsigned short uint16;
typedef unsigned int  uint32;
typedef unsigned long uint64;

// ilp32e: unsigned long is 4 bytes, so "uint64" is a 32-bit type in this
// build, on purpose. struct stat and other ABI structs were laid out with it
// (the Phase 1.7 Gate D lesson: size at offset 12), so the typedef stays and
// this assert makes the width explicit. CSR helpers in riscv.h use uint32.
// See S-ASCEND-2 Phase 7.
_Static_assert(sizeof(uint64) == 4, "ilp32e: uint64 is 4 bytes here on purpose");

typedef uint64 pde_t; // Sv32 page-directory entry: 4 bytes
