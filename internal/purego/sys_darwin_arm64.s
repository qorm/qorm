//go:build darwin && arm64

#include "textflag.h"

TEXT ·libc_dlopen_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_dlopen(SB)

TEXT ·libc_dlsym_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_dlsym(SB)

TEXT ·dlopenTrampoline(SB),NOSPLIT,$0-8
	MOVD	$·libc_dlopen_trampoline(SB), R0
	MOVD	R0, ret+0(FP)
	RET

TEXT ·dlsymTrampoline(SB),NOSPLIT,$0-8
	MOVD	$·libc_dlsym_trampoline(SB), R0
	MOVD	R0, ret+0(FP)
	RET
