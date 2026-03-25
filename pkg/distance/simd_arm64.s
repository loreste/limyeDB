#include "textflag.h"

// ARM64 optimized distance calculations using Go Plan 9 assembly
// Uses loop unrolling for better performance on ARM64

// func dotProductNEON(a, b []float32) float32
TEXT ·dotProductNEON(SB), NOSPLIT, $0-52
    MOVD a_base+0(FP), R0      // R0 = &a[0]
    MOVD a_len+8(FP), R1       // R1 = len(a)
    MOVD b_base+24(FP), R2     // R2 = &b[0]

    // Initialize accumulators
    FMOVS $0.0, F0             // F0 = sum0
    FMOVS $0.0, F4             // F4 = sum1
    FMOVS $0.0, F5             // F5 = sum2
    FMOVS $0.0, F6             // F6 = sum3

    // Calculate number of 4-element chunks
    LSR $2, R1, R3             // R3 = len / 4
    CBZ R3, dot_remainder

dot_loop_4:
    // Load 4 elements from each array
    FMOVS (R0), F1
    FMOVS 4(R0), F2
    FMOVS 8(R0), F3
    FMOVS 12(R0), F7

    FMOVS (R2), F8
    FMOVS 4(R2), F9
    FMOVS 8(R2), F10
    FMOVS 12(R2), F11

    // Multiply and accumulate
    FMADDS F8, F1, F0, F0      // F0 += F1 * F8
    FMADDS F9, F2, F4, F4      // F4 += F2 * F9
    FMADDS F10, F3, F5, F5     // F5 += F3 * F10
    FMADDS F11, F7, F6, F6     // F6 += F7 * F11

    // Advance pointers
    ADD $16, R0
    ADD $16, R2

    SUBS $1, R3
    BNE dot_loop_4

    // Combine accumulators
    FADDS F4, F0, F0
    FADDS F5, F0, F0
    FADDS F6, F0, F0

dot_remainder:
    // Handle remaining elements (len % 4)
    AND $3, R1, R4
    CBZ R4, dot_done

dot_scalar_loop:
    FMOVS (R0), F1
    FMOVS (R2), F2
    FMADDS F2, F1, F0, F0

    ADD $4, R0
    ADD $4, R2
    SUBS $1, R4
    BNE dot_scalar_loop

dot_done:
    FMOVS F0, ret+48(FP)
    RET


// func euclideanDistanceNEON(a, b []float32) float32
TEXT ·euclideanDistanceNEON(SB), NOSPLIT, $0-52
    MOVD a_base+0(FP), R0
    MOVD a_len+8(FP), R1
    MOVD b_base+24(FP), R2

    // Initialize accumulators
    FMOVS $0.0, F0
    FMOVS $0.0, F4
    FMOVS $0.0, F5
    FMOVS $0.0, F6

    LSR $2, R1, R3
    CBZ R3, euc_remainder

euc_loop_4:
    // Load 4 elements
    FMOVS (R0), F1
    FMOVS 4(R0), F2
    FMOVS 8(R0), F3
    FMOVS 12(R0), F7

    FMOVS (R2), F8
    FMOVS 4(R2), F9
    FMOVS 8(R2), F10
    FMOVS 12(R2), F11

    // Compute differences
    FSUBS F8, F1, F1
    FSUBS F9, F2, F2
    FSUBS F10, F3, F3
    FSUBS F11, F7, F7

    // Square and accumulate
    FMADDS F1, F1, F0, F0
    FMADDS F2, F2, F4, F4
    FMADDS F3, F3, F5, F5
    FMADDS F7, F7, F6, F6

    ADD $16, R0
    ADD $16, R2
    SUBS $1, R3
    BNE euc_loop_4

    // Combine accumulators
    FADDS F4, F0, F0
    FADDS F5, F0, F0
    FADDS F6, F0, F0

euc_remainder:
    AND $3, R1, R4
    CBZ R4, euc_sqrt

euc_scalar_loop:
    FMOVS (R0), F1
    FMOVS (R2), F2
    FSUBS F2, F1, F1
    FMADDS F1, F1, F0, F0

    ADD $4, R0
    ADD $4, R2
    SUBS $1, R4
    BNE euc_scalar_loop

euc_sqrt:
    FSQRTS F0, F0
    FMOVS F0, ret+48(FP)
    RET


// func cosineDistanceNEON(a, b []float32) float32
TEXT ·cosineDistanceNEON(SB), NOSPLIT, $0-52
    MOVD a_base+0(FP), R0
    MOVD a_len+8(FP), R1
    MOVD b_base+24(FP), R2

    // Initialize: dot product (F0), normA (F12), normB (F13)
    FMOVS $0.0, F0
    FMOVS $0.0, F4
    FMOVS $0.0, F5
    FMOVS $0.0, F6
    FMOVS $0.0, F12
    FMOVS $0.0, F13
    FMOVS $0.0, F14
    FMOVS $0.0, F15
    FMOVS $0.0, F16
    FMOVS $0.0, F17
    FMOVS $0.0, F18
    FMOVS $0.0, F19

    LSR $2, R1, R3
    CBZ R3, cos_remainder

cos_loop_4:
    // Load 4 elements
    FMOVS (R0), F1
    FMOVS 4(R0), F2
    FMOVS 8(R0), F3
    FMOVS 12(R0), F7

    FMOVS (R2), F8
    FMOVS 4(R2), F9
    FMOVS 8(R2), F10
    FMOVS 12(R2), F11

    // Dot product: a[i] * b[i]
    FMADDS F8, F1, F0, F0
    FMADDS F9, F2, F4, F4
    FMADDS F10, F3, F5, F5
    FMADDS F11, F7, F6, F6

    // NormA: a[i] * a[i]
    FMADDS F1, F1, F12, F12
    FMADDS F2, F2, F14, F14
    FMADDS F3, F3, F15, F15
    FMADDS F7, F7, F16, F16

    // NormB: b[i] * b[i]
    FMADDS F8, F8, F13, F13
    FMADDS F9, F9, F17, F17
    FMADDS F10, F10, F18, F18
    FMADDS F11, F11, F19, F19

    ADD $16, R0
    ADD $16, R2
    SUBS $1, R3
    BNE cos_loop_4

    // Combine accumulators
    FADDS F4, F0, F0
    FADDS F5, F0, F0
    FADDS F6, F0, F0

    FADDS F14, F12, F12
    FADDS F15, F12, F12
    FADDS F16, F12, F12

    FADDS F17, F13, F13
    FADDS F18, F13, F13
    FADDS F19, F13, F13

cos_remainder:
    AND $3, R1, R4
    CBZ R4, cos_compute

cos_scalar_loop:
    FMOVS (R0), F1
    FMOVS (R2), F2

    FMADDS F2, F1, F0, F0      // dot
    FMADDS F1, F1, F12, F12    // normA
    FMADDS F2, F2, F13, F13    // normB

    ADD $4, R0
    ADD $4, R2
    SUBS $1, R4
    BNE cos_scalar_loop

cos_compute:
    // Check for zero norms
    FMOVS $0.0, F3
    FCMPS F12, F3
    BEQ cos_zero_norm
    FCMPS F13, F3
    BEQ cos_zero_norm

    // similarity = dot / (sqrt(normA) * sqrt(normB))
    FSQRTS F12, F12
    FSQRTS F13, F13
    FMULS F13, F12, F12
    FDIVS F12, F0, F0

    // distance = 1.0 - similarity
    FMOVS $1.0, F3
    FSUBS F0, F3, F3
    FMOVS F3, ret+48(FP)
    RET

cos_zero_norm:
    FMOVS $1.0, F3
    FMOVS F3, ret+48(FP)
    RET
