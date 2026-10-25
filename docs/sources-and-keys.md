# Sources and keys

KLOS uses only official, verifiable sources.

## Rules for sources

1. Each source has the official download address of the upstream project.
2. Each source has a pinned SHA-256 value in its recipe. kbuild checks it
   when it downloads the file and again before each build.
3. If upstream signs its releases, the recipe also has the signature and
   the key. kbuild checks the signature when it downloads the file.
4. A source without an upstream signature has a comment in its recipe.

## Keys

The `keys/` directory has the public keys. The file name is the key name
in the recipes (`key = ["musl"]` is `keys/musl.asc`).

| Key file | Owner | Fingerprint | Origin |
|----------|-------|-------------|--------|
| alpine-ncopa.asc | Natanael Copa (Alpine) | 0482 D840 22F5 2DF1 C4E7 CD43 293A CD09 07D9 495A | alpinelinux.org/keys |
| llvm.asc | LLVM release managers | D574 BD5D 1D0E 9889 5E3B F900 44F2 485E 45D5 9042 (and others) | releases.llvm.org/release-keys.asc |
| musl.asc | musl libc | 8364 8929 0BB6 B70F 99FF DA05 56BC DB59 3020 450F | musl.libc.org/musl.pub |
| kernel-gregkh.asc | Greg Kroah-Hartman | 647F 2865 4894 E3BD 4571 99BE 38DB BDC8 6092 693E | kernel.org pgpkeys |
| kernel-torvalds.asc | Linus Torvalds | ABAF 11C6 5A29 70B1 30AB E3C4 79BE 3E43 0041 1886 | kernel.org pgpkeys |
| util-linux-kzak.asc | Karel Zak | B0C6 4D14 301C C6EF AEDF 60E4 E4B7 1D5E EC39 C284 | kernel.org pgpkeys |
| cryptsetup-mbroz.asc | Milan Broz | 2A29 1824 3FDE 4664 8D06 86F9 D9B0 577B D93E 98FC | kernel.org pgpkeys |
| e2fsprogs-tytso.asc | Theodore Ts'o | 2B69 B954 DBFE 0879 2881 37C9 F2F9 5956 950D 81A3 | keyserver.ubuntu.com |
| iproute2.asc | Stephen Hemminger | 110B 8614 708F 62B8 3DD1 9F23 F028 172E C6AE B2B3 | keyserver.ubuntu.com |
| openssh.asc | OpenSSH (Damien Miller) | 7168 B983 815A 5EEF 59A4 ADFD 2A3F 414E 7360 60BA | cdn.openbsd.org |
| chrony-mlichvar.asc | Miroslav Lichvar | 8F37 5C7E 8D0E E125 A3D3 BD51 537E 2B76 F768 0DAC | keyserver.ubuntu.com |
| netfilter-pablo.asc | Netfilter Core Team | 8C5F 7146 A175 7A65 E242 2A94 D70D 1A66 6ACF 2B21 | keyserver.ubuntu.com |
| netfilter-libmnl.asc | Netfilter Core Team | 37D9 64AC C049 81C7 5500 FB9B D55D 978A 8A14 20E4 | keyserver.ubuntu.com |
| dhcpcd-roy.asc | Roy Marples | A785 ED27 5595 5D9E 93EA 59F6 597F 97EA 9AD4 5549 | keyserver.ubuntu.com |
| dosfstools.asc | Andreas Bombe | FDF2 7714 0B6D F9E8 CDC8 A982 3164 705B 1DA3 1D4C | keyserver.ubuntu.com |
| lvm2.asc | Marian Csontos | D501 A478 440A E2FD 130A 1BE8 B911 2431 E509 039F | keyserver.ubuntu.com |
| zlib-madler.asc | Mark Adler | 5ED4 6A67 21D3 6558 7791 E2AA 783F CD8E 58BC AFBA | keyserver.ubuntu.com |
| libcap-morgan.asc | Andrew G. Morgan | 38A6 4469 8C69 7873 44E9 54CE 29EE 848A E2CC F3F4 | kernel.org pgpkeys |
| libseccomp.asc | Tom Hromatka (Oracle) | 47A6 8FCE 37C7 D702 4FD6 5E11 356C E62C 2B52 4099 | keyserver.ubuntu.com |
| crun-gscrivano.asc | Giuseppe Scrivano | AC40 4C1C 0BF7 35C6 3FF4 D562 263D 6DF2 E163 E1EA | keyserver.ubuntu.com |
| kmod-ldm.asc | Lucas De Marchi | EAB3 3C96 9001 3C73 3916 AC83 9BA2 A5A6 30CB EA53 | kernel.org pgpkeys |
| zstd.asc | Zstandard Release Signing Key | 4EF4 AC63 455F C9F4 545D 9B7D EF8F E995 28B5 2FFD | keyserver.ubuntu.com |
| freedesktop-emersion.asc | Simon Ser | 34FF 9526 CFEF 0E97 A340 E2E4 0FDE 7BE0 E88F 5E48 | keyserver.ubuntu.com |
| pixman-cairo.asc | Matt Turner | 3BB6 39E5 6F86 1FA2 E865 0569 0FDD 682D 974C A72A | keyserver.ubuntu.com |
| cairo-ebassi.asc | Emmanuele Bassi | 53EF 3DC3 B63E 2899 271B D263 22E8 091E EA11 BBB7 | keyserver.ubuntu.com, compared with keys.openpgp.org and gitlab.gnome.org |
| mesa-release.asc | Eric Engestrom | 5755 1DE1 5B96 8F63 41C2 48F6 8D8E 31AF C324 28A6 | keyserver.ubuntu.com |
| libevdev-whot.asc | Peter Hutterer (Who-T) | 3C2C 43D9 447D 5938 EF45 51EB E23B 7E70 B467 F0BF | keyserver.ubuntu.com |
| xkeyboard-config.asc | Sergey Udaltsov | FFB4 CCD2 75AA A422 F5F9 808E 0661 D98F C933 A145 | keyserver.ubuntu.com |
| pcre2.asc | Nicholas Wilson | A955 3620 4A3B B489 7152 3128 2A98 E77E B6F2 4CA8 | keyserver.ubuntu.com |
| expat-sping.asc | Sebastian Pipping | 3176 EF7D B236 7F1F CA4F 306B 1F9B 0E90 9AF3 7285 | keyserver.ubuntu.com |
| freetype-wl.asc | Werner Lemberg | E306 7470 7856 409F F194 8010 BE6C 3AAC 63AD 8E3F | keyserver.ubuntu.com |
| dnkl.asc | Daniel Eklöf (Git signing) | B199 64FB BA09 664C C810 27ED 5BBD 4992 C116 573F | keyserver.ubuntu.com |

Before you add a key, compare its fingerprint with a second source. For
example, use the web site of the project, the release announcement, or the
kernel.org key ring.

## Sources without signatures

These upstream projects do not sign their releases. Their recipes have
only a pinned SHA-256 value:

- toybox, dash, dinit, popt, json-c, libaio, argp-standalone, mtdev,
  libudev-zero, seatd, libinput, libxkbcommon, libffi, fribidi, harfbuzz,
  utf8proc, tllist, hwdata, libpng, the DejaVu fonts, containerd, the CNI plugins,
  cri-tools (upstream publishes no signatures, or only signed Git tags)
- glib, pango, fontconfig, libaio (upstream publishes SHA-256 or SHA-512
  values; the pinned values agree with them)
- Kubernetes (dl.k8s.io publishes the SHA-256 value and Sigstore signatures;
  kbuild does not check Sigstore signatures yet)
- the Python build modules (PyPI publishes the SHA-256 values)
- the Mozilla CA bundle from curl.se (served with HTTPS only)
- Go (go.dev publishes the SHA-256 values with HTTPS)

## The trust chain

1. The host verifies the Alpine minirootfs (SHA-256 and OpenPGP signature).
2. apk in the builder verifies each Alpine package with the Alpine keys in
   the minirootfs.
3. kbuild verifies each source (SHA-256 and the signature, if available).
4. kbuild signs each update bundle with the KLOS key (Ed25519).
5. The installed system verifies the update bundles with its public key,
   and dm-verity verifies each block of the OS image when it reads it.
