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
| xkeyboard-config.asc | Sergey Udaltsov | FFB4 CCD2 75AA A422 F5F9 808E 0661 D98F C933 A145 | keyserver.ubuntu.com |
| pcre2.asc | Nicholas Wilson | A955 3620 4A3B B489 7152 3128 2A98 E77E B6F2 4CA8 | keyserver.ubuntu.com |
| expat-sping.asc | Sebastian Pipping | 3176 EF7D B236 7F1F CA4F 306B 1F9B 0E90 9AF3 7285 | keyserver.ubuntu.com |
| freetype-wl.asc | Werner Lemberg | E306 7470 7856 409F F194 8010 BE6C 3AAC 63AD 8E3F | keyserver.ubuntu.com |
| dnkl.asc | Daniel Eklöf (Git signing) | B199 64FB BA09 664C C810 27ED 5BBD 4992 C116 573F | keyserver.ubuntu.com |
| archlinux-pierre.asc | Pierre Schmitz (Arch Linux releases) | 3E80 CA1A 8B89 F69C BA57 D98A 76A5 EF90 5444 9A5C | keyserver.ubuntu.com, compared with archlinux.org/download and pierre-schmitz.com |
| selinux-lautrbach.asc | Petr Lautrbach (SELinux userspace) | 68D2 1823 342A 1368 3AEB 3E4E FB4C 685B 5DC1 C13E | keyserver.ubuntu.com, compared with keys.openpgp.org and github.com/bachradsusi |
| openssl.asc | OpenSSL | B146 647E 45A7 B339 47AB 226B 2A2C 87D1 6169 2D40 | keyserver.ubuntu.com, compared with openssl-library.org |
| python-wouters.asc | Thomas Wouters (Python 3.12 and 3.13) | 7169 605F 62C7 5135 6D05 4A26 A821 E680 E5FA 6305 | keyserver.ubuntu.com, compared with keys.openpgp.org and python.org |
| cmake-bking.asc | Brad King (CMake) | CBA2 3971 357C 2E65 90D9 EFD3 EC8F EF3A 7BFB 4EDA | keyserver.ubuntu.com, compared with keys.openpgp.org |
| meson-jpakkane.asc | Jussi Pakkanen (Meson) | 19E2 D6D9 B46D 8DAA 6288 F877 C24E 631B ABB1 FE70 | keyserver.ubuntu.com, compared with keys.openpgp.org |
| xz-lasse.asc | Lasse Collin (XZ Utils) | 3690 C240 CE51 B467 0D30 AD1C 38EE 757D 6918 4620 | keyserver.ubuntu.com, compared with keys.openpgp.org |
| bzip2-wielaard.asc | Mark Wielaard (bzip2) | EC3C FE88 F6CA 0788 774F 5C1D 1AA4 4BE6 49DE 760A | keyserver.ubuntu.com, compared with keys.openpgp.org |
| file-christos.asc | Christos Zoulas (file) | BE04 995B A8F9 0ED0 C0C1 76C4 7111 2AB1 6CB3 3B3A | keyserver.ubuntu.com, compared with keys.openpgp.org |
| flex-estes.asc | Will Estes (flex) | 56C6 7868 E933 90AA 1039 AD1C E4B2 9C8D 6488 5307 | keyserver.ubuntu.com (no second source; the release signature agrees) |
| gnu-chet.asc | Chet Ramey (bash) | 7C01 35FB 088A AF6C 66C6 50B9 BB58 69F0 64EA 74AB | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-pbrady.asc | Pádraig Brady (coreutils) | 6C37 DC12 121A 5006 BC1D B804 DF6F D971 3060 37D9 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-meyering.asc | Jim Meyering (sed, grep, diffutils, gzip) | 155D 3FC5 00C8 3448 6D1E EA67 7FD9 FCCB 000B EEEE | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-arnold.asc | Arnold Robbins (gawk) | D196 7C63 7887 1317 7D86 1ED7 DF59 7815 937E C0D2 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-youngman.asc | James Youngman (findutils) | 0CF4 E8D8 7159 3224 8428 32B8 88DD 9E08 C5DD ACB9 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-agruen.asc | Andreas Gruenbacher (patch) | 259B 3792 B3D6 D319 212C C4DC D5BF 9FEB 0313 653A | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-gray.asc | Sergey Poznyakoff (tar) | 325F 650C 4C2B 6AD5 8807 327A 3602 B07F 55D0 C732 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-psmith.asc | Paul D. Smith (make) | 6D4E EB02 AD83 4703 510B 1176 80CB 727A 20C7 9BB2 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-eblake.asc | Eric Blake (m4) | 71C2 CC22 B1C4 6029 27D2 F3AA A7A1 6B4A 2527 436A | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-demaille.asc | Akim Demaille (bison) | 7DF8 4374 B1EE 1F97 64BB E25D 0DDC AA32 78D5 264E | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-zackw.asc | Zack Weinberg (autoconf) | 82F8 54F3 CE73 174B 8B63 1740 91FC C32B 6769 AA64 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-iczelia.asc | Kamila Szewczyk (automake) | 6C22 2EA6 B2BD 216A A406 516A C868 F0B6 DE38 409D | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-ileana.asc | Ileana Dumitrescu (libtool) | FA26 CA78 4BE1 8892 7F22 B99F 6570 EA01 146F 7354 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-gavin.asc | Gavin Smith (texinfo) | EAF6 69B3 1E31 E1DE CBD1 1513 DDBC 579D AB37 FBA9 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-pizzini.asc | Ken Pizzini (bc) | 325B 54E4 653F 935A 25CF 32BD A186 278D 426A 38E9 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-haible.asc | Bruno Haible (gperf) | E0FF BD97 5397 F77A 32AB 76EC B630 1D9E 1BBE AC08 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |
| gnu-knaff.asc | Alain Knaff (mtools) | 51A2 3D9D 7C4D F41A A48F A893 E99C F553 7790 B839 | the GNU keyring (ftp.gnu.org/gnu/gnu-keyring.gpg), compared with keyserver.ubuntu.com |

Before you add a key, compare its fingerprint with a second source. For
example, use the web site of the project, the release announcement, or the
kernel.org key ring.

The GNU packages come from `ftpmirror.gnu.org`, which sends the request to
a GNU mirror. The signature and the SHA-256 value make sure that the file
is the official release.

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
- libevdev (upstream signs with a DSA-1024 key; kbuild does not accept
  keys that weak)
- Perl and lz4 (upstream publishes SHA-256 values; the pinned values agree
  with them)
- samurai, pkgconf (upstream does not sign the release tarballs)
- erofs-utils (upstream signs the Git tags; the snapshot tarball of
  git.kernel.org has no signature)

## The trust chain

1. The host verifies the Alpine minirootfs (SHA-256 and OpenPGP signature).
2. apk in the builder verifies each Alpine package with the Alpine keys in
   the minirootfs.
3. kbuild verifies each source (SHA-256 and the signature, if available).
4. kbuild signs each update bundle with the KLOS key (Ed25519).
5. The installed system verifies the update bundles with its public key,
   and dm-verity verifies each block of the OS image when it reads it.
