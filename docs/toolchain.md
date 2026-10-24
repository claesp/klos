# Toolchain and C library

## Stages

The KLOS toolchain is the host package `llvm` (`pkgs/llvm/recipe.toml`).

1. The bootstrap compiler (Clang from Alpine) compiles LLVM, Clang and lld.
   The new compiler has these defaults:
   - target `x86_64-unknown-linux-musl`
   - run-time library `compiler-rt` (not libgcc)
   - unwinder `libunwind`
   - C++ library `libc++`
   - linker `lld`
2. The new compiler compiles the compiler-rt builtins and the CRT start
   files (`crtbegin.o`, `crtend.o`) for the target.
3. The new compiler compiles musl (`pkgs/musl`).
4. The new compiler compiles libunwind, libc++abi and libc++
   (`pkgs/llvm-runtimes`). libc++ uses the "fast" hardening mode.
5. The new compiler compiles all other target packages.

No compiler from Alpine compiles target code. The Alpine GCC compiles only
build-time helper programs (for example the kconfig tools of the kernel).

## The C library

KLOS uses musl. musl is small, has a clear code base, and supports static
and dynamic linking. The dynamic linker is `/usr/lib/ld-musl-x86_64.so.1`.

## Default flags

The flags are in `config/klos.toml`:

| Flag | Function |
|------|----------|
| `-O2 -march=x86-64-v2` | Optimization for x86-64-v2 processors (2009 and later). |
| `-fstack-protector-strong` | Stack canaries. |
| `-fstack-clash-protection` | Stack clash protection. |
| `-ftrivial-auto-var-init=zero` | Automatic variables start at zero. |
| `-fcf-protection=full` | Intel CET (shadow stack and indirect branch tracking). |
| `-D_FORTIFY_SOURCE=2` | Buffer checks in the C library functions, where available. |
| `-Wl,-z,relro -Wl,-z,now` | Full RELRO. |
| `-Wl,-z,noexecstack` | No executable stack. |
| `-Wl,-z,pack-relative-relocs` | Compact relocations (RELR). |
| `-fPIE -pie` | Position-independent executables (the Clang default). |

## LLVM for the target

The package `llvm-target` is a second LLVM build, for KLOS itself (not for
the builder). It supplies `libLLVM.so` (with the X86 and AMDGPU back ends)
and the clang libraries. Mesa uses it for llvmpipe and radeonsi, and for
`mesa_clc`, which compiles the OpenCL C kernels of the Intel drivers.

The build has two passes:

1. Build the tools that a cross build of LLVM runs (`llvm-tblgen`,
   `clang-tblgen`, `llvm-config`, `llvm-nm`, ...) as static programs. They
   run in the builder.
2. Build LLVM and clang for the target, with `LLVM_NATIVE_TOOL_DIR` set to
   the tools of pass 1.

`mesa_clc` is a target program. Meson runs it in the builder through the
`exe_wrapper` of the kbuild cross file (the KLOS dynamic linker with the
sysroot libraries). The desktop image gets only `libLLVM.so`; the clang
libraries and `llvm-config` stay out of the image.

## Go programs

The host package `go` is the official Go release. Go programs in KLOS are
static (`CGO_ENABLED=0`) and use `-trimpath` for reproducible builds.
