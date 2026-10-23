# SPDX-License-Identifier: BSD-2-Clause
# Helper functions for recipe build scripts. kbuild loads this file first.
# The environment comes from kbuild: S, DESTDIR, SYSROOT, TARGET, JOBS, ...

# Run an autoconf "configure" script for the target.
kconfigure() {
	"${CONFIGURE:-./configure}" \
		--build="$BUILD_TRIPLE" --host="$TARGET" \
		--prefix=/usr --sysconfdir=/etc --localstatedir=/var \
		--sbindir=/usr/bin --libexecdir=/usr/lib --mandir=/usr/share/man \
		--disable-nls "$@"
}

kmake() {
	make -j"$JOBS" V=1 "$@"
}

# Configure a CMake project for the target in the "build" directory.
kcmake() {
	cmake -S "${CMAKE_SOURCE:-.}" -B build -G Ninja \
		-DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN_FILE" \
		-DCMAKE_BUILD_TYPE=None \
		-DCMAKE_INSTALL_PREFIX=/usr \
		-DCMAKE_INSTALL_LIBDIR=lib \
		-DCMAKE_INSTALL_SBINDIR=bin \
		-DCMAKE_INSTALL_LIBEXECDIR=lib \
		-DCMAKE_C_FLAGS_INIT="-DNDEBUG" \
		-DCMAKE_CXX_FLAGS_INIT="-DNDEBUG" \
		"$@"
}

# Configure a Meson project for the target in the "build" directory.
kmeson() {
	meson setup build "${MESON_SOURCE:-.}" \
		--cross-file="$MESON_CROSS_FILE" \
		--prefix=/usr --libdir=lib --sbindir=bin --libexecdir=lib \
		--sysconfdir=/etc --localstatedir=/var \
		--buildtype=plain --wrap-mode=nodownload \
		-Db_pie=true -Db_ndebug=true \
		"$@"
}

kninja() {
	ninja -C build -j"$JOBS" "$@"
}

kninja_install() {
	DESTDIR="$DESTDIR" ninja -C build install
}

# Install a file with a mode: kinstall MODE SOURCE DEST
kinstall() {
	install -D -m "$1" "$2" "$DESTDIR$3"
}
