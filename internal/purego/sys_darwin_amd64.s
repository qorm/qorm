//go:build darwin && amd64

#include "textflag.h"

TEXT ·libc_dlopen_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_dlopen(SB)

TEXT ·libc_dlsym_trampoline(SB),NOSPLIT,$0-0
	JMP	libc_dlsym(SB)

TEXT ·dlopenTrampoline(SB),NOSPLIT,$0-8
	LEAQ	·libc_dlopen_trampoline(SB), AX
	MOVQ	AX, ret+0(FP)
	RET

TEXT ·dlsymTrampoline(SB),NOSPLIT,$0-8
	LEAQ	·libc_dlsym_trampoline(SB), AX
	MOVQ	AX, ret+0(FP)
	RET
