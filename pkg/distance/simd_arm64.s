#include "textflag.h"

// func dotProductNEON(a, b []float32) float32
TEXT ·dotProductNEON(SB), NOSPLIT, $0-52
    MOVD a_base+0(FP), R0
    MOVD a_len+8(FP), R1
    MOVD b_base+24(FP), R2

    FSUBS F0, F0, F0
    MOVD $0, R3

dot_scalar_loop:
    CMP R3, R1
    BGE dot_done

    LSL $2, R3, R4
    ADD R0, R4, R5
    ADD R2, R4, R6

    FMOVS (R5), F1
    FMOVS (R6), F2
    FMULS F2, F1, F1
    FADDS F1, F0, F0

    ADD $1, R3
    B dot_scalar_loop

dot_done:
    FMOVS F0, ret+48(FP)
    RET


// func euclideanDistanceNEON(a, b []float32) float32
TEXT ·euclideanDistanceNEON(SB), NOSPLIT, $0-52
    MOVD a_base+0(FP), R0
    MOVD a_len+8(FP), R1
    MOVD b_base+24(FP), R2

    FSUBS F0, F0, F0
    MOVD $0, R3

euc_scalar_loop:
    CMP R3, R1
    BGE euc_done

    LSL $2, R3, R4
    ADD R0, R4, R5
    ADD R2, R4, R6

    FMOVS (R5), F1
    FMOVS (R6), F2
    FSUBS F2, F1, F1
    FMULS F1, F1, F1
    FADDS F1, F0, F0

    ADD $1, R3
    B euc_scalar_loop

euc_done:
    FSQRTS F0, F0
    FMOVS F0, ret+48(FP)
    RET


// func cosineDistanceNEON(a, b []float32) float32
TEXT ·cosineDistanceNEON(SB), NOSPLIT, $0-52
    MOVD a_base+0(FP), R0
    MOVD a_len+8(FP), R1
    MOVD b_base+24(FP), R2

    FSUBS F0, F0, F0 // dot
    FSUBS F1, F1, F1 // normA
    FSUBS F2, F2, F2 // normB

    MOVD $0, R3

cos_scalar_loop:
    CMP R3, R1
    BGE cos_done

    LSL $2, R3, R4
    ADD R0, R4, R5
    ADD R2, R4, R6

    FMOVS (R5), F3
    FMOVS (R6), F4

    FMULS F4, F3, F5
    FADDS F5, F0, F0

    FMULS F3, F3, F6
    FADDS F6, F1, F1

    FMULS F4, F4, F7
    FADDS F7, F2, F2

    ADD $1, R3
    B cos_scalar_loop

cos_done:
    FSUBS F3, F3, F3
    FCMPS F1, F3
    BEQ zero_norm
    FCMPS F2, F3
    BEQ zero_norm

    FSQRTS F1, F1
    FSQRTS F2, F2
    FMULS F2, F1, F1
    FDIVS F1, F0, F0

    // 1.0 - similarity
    FMOVS $1.0, F3
    FSUBS F0, F3, F3

    FMOVS F3, ret+48(FP)
    RET

zero_norm:
    FMOVS $1.0, F3
    FMOVS F3, ret+48(FP)
    RET
