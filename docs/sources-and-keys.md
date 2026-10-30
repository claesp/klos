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
| rust.asc | Rust Language (Tag and Release Signing Key) | 108F 6620 5EAE B0AA A8DD 5E1C 85AB 96E6 FA1B E5FE | static.rust-lang.org/rust-key.gpg.ascii, compared with keys.openpgp.org |
| google-linux.asc | Google Linux Packages Signing Authority (Go) | EB4C 1BFD 4F04 2F6D DDCC EC91 7721 F63B D38B 4796 | dl.google.com/dl/linux/linux_signing_key.pub |
| stefanberger.asc | Stefan Berger (libtpms, swtpm) | B818 B9CA DF90 89C2 D5CE C66B 75AD 6580 2A0B 4211 | github.com/stefanberger.gpg |
| perl-pause.asc | PAUSE Batch Signing Key (CPAN) | 2E66 557A B97C 19C7 91AF 8E20 328D A867 450F 89EC | keyserver.ubuntu.com, compared with pause.perl.org |
| containerd-skarp.asc | Samuel Karp (containerd tags) | 910C 2860 8D33 DDE6 89C0 3290 997C 5A3C D316 7CB5 | github.com/samuelkarp.gpg |
| fribidi-dov.asc | Dov Grobgeld (fribidi tags) | 0AD0 41B2 7CA1 66DD A1FE 3BAE A7B3 409C 0CA4 ED14 | github.com/dov.gpg |
| harfbuzz-khaledhosny.asc | Khaled Hosny (HarfBuzz tags) | 053D 20F1 7CCC A965 1B2C 6FCB 9AB2 4930 C0B9 97A2 | github.com/khaledhosny.gpg |
| hwdata-vcrhonek.asc | Vitezslav Crhonek (hwdata tags) | 3C40 194F B791 38CE 0F78 FD49 19C2 F062 574F 5403 | keyserver.ubuntu.com |
| erofs-hsiangkao.asc | Gao Xiang (erofs-utils tags) | 4340 3A6C 3512 F58F BCDC D3C5 5176 67E5 996E E6AA | keys.openpgp.org |
| fontconfig-tagoh.asc | Akira TAGOH (fontconfig tags) | ECFF BC3A 6B36 5E72 1E5B D798 3075 7AA2 1971 A672 | gitlab.freedesktop.org/tagoh.gpg |
| gnome-pwithnall.asc | Philip Withnall (GLib tags) | 923B 7025 EE03 C1C5 9F42 684C F094 2E89 4B2E AFA0 | keyserver.ubuntu.com |
| slirp-sthibault.asc | Samuel Thibault (libslirp tags) | 900C B024 B679 31D4 0F82 304B D017 8C76 7D06 9EE6 | keys.openpgp.org |
| libpng-ctruta.asc | Cosmin Truta (libpng tags) | 1FED 507E 3236 B4A6 E53E 922B B292 C648 43FF 5BCF | github.com/ctruta.gpg |
| alsa-release.asc | ALSA Release Team | F04D F507 37AC 1A88 4C4B 3D71 8380 596D A6E5 9C91 | keyserver.ubuntu.com |
| dbus-smcv.asc | Simon McVittie (D-Bus) | DA98 F25C 0871 C49A 59EA FF2C 4DE8 FF2A 63C7 CC90 | keyserver.ubuntu.com |
| jansson-akheron.asc | Petri Lehtinen (jansson) | B5D6 953E 6D50 59ED 7ADA 0F2F D365 7D24 D058 434C | github.com/akheron.gpg |
| etcd-ivanvc.asc | Ivan Valdes (etcd tags) | 67A9 5D46 D983 86A2 D31E F773 4037 D377 41ED 0CC5 | github.com/ivanvc.gpg |
| atspi-mgorse.asc | Mike Gorse (at-spi2-core tags) | 276B FDAF B362 12F8 DB09 5E21 4082 3A05 6FD6 D4B3 | keyserver.ubuntu.com |
| ncurses-dickey.asc | Thomas E. Dickey (ncurses) | 1988 2D92 DDA4 C400 C22C 0D56 CC2A F447 2167 BE03 | invisible-island.net, compared with keyserver.ubuntu.com |
| nodejs-aduh95.asc | Antoine du Hamel (Node.js releases) | 5BE8 A3F6 C8A5 C01D 106C 0AD8 20B1 A390 B168 D356 | github.com/nodejs/release-keys, compared with the README of Node.js |
| mozilla-releases.asc | Mozilla Software Releases | 14F2 6682 D091 6CDD 81E3 7B6D 61B7 B526 D98F 0353 | archive.mozilla.org (KEY), compared with keys.openpgp.org |
| sigstore-trusted-root.json | Sigstore (Fulcio CA and Rekor log keys) | SHA-256 6494 e21e a73f a7ee 769f 85f5 7d5a 3e6a 0872 5eae 1e38 c755 fc35 17c9 e6bc 0b66 | github.com/sigstore/root-signing (targets/trusted_root.json) |
| klos-seed.pub | The KLOS seed key (minisign) | key ID 59FA6379CFDC7319 | this repository (`./klos seed-keygen`) |

Before you add a key, compare its fingerprint with a second source. For
example, use the web site of the project, the release announcement, or the
kernel.org key ring.

The keys of the signed Git tags are the keys of the persons that tagged
the release. Another maintainer can tag the next release: then add the key
of that person.

The PAUSE key of CPAN gets a new signing subkey each year. Get
`perl-pause.asc` again when the subkey expires.

The GNU packages come from `ftpmirror.gnu.org`, which sends the request to
a GNU mirror. The signature and the SHA-256 value make sure that the file
is the official release.

## Signature types

The `signature-type` of a source tells kbuild how to check it:

| Type | Check |
|------|-------|
| `pgp` (default) | A detached OpenPGP signature of the file (`signature`, `key`). |
| `pgp-checksums` | A signed OpenPGP message (also a cleartext signed message) with the SHA-256 or SHA-512 value of the file. kbuild also reads the CHECKSUMS files of CPAN. |
| `pgp-checksum-file` | A detached OpenPGP signature of a checksum list (`checksums`). |
| `minisign` | A minisign signature (`keys/<name>.pub`). |
| `sigstore` | A Sigstore signature of the file (`signature`, `certificate`). Read "Sigstore" below. |
| `pypi-attestation` | A PyPI attestation (PEP 740) of the file. Read "Sigstore" below. |
| `git-tag` | A signed Git tag (`git`, `tag`, `key`). Read "Signed Git tags" below. |

kbuild checks each signature first at the current time, then at the time
of the signature. Thus a key that expired after the release still checks
the release. `allow-expired-key = true` accepts a key that expired before
the signature (the recipe must tell why).

### Sigstore

Sigstore gives a short-lived certificate (Fulcio) for an identity: an
e-mail address or a CI workflow. The transparency log (Rekor) records each
signature. kbuild checks these items:

1. The signature of the file, or of the in-toto statement that names the
   file and its SHA-256 value (`pypi-attestation`).
2. The certificate chain to the Fulcio CA in `keys/sigstore-trusted-root.json`.
3. The identity (`identity`) and the OIDC issuer (`issuer`) in the
   certificate.
4. The signed entry timestamp of the Rekor entry, with the Rekor key in
   the trusted root. The log time must be in the validity of the
   certificate.

For a `sigstore` source, kbuild gets the Rekor entry from the Rekor API.
A PyPI attestation contains the Rekor entry.

### Signed Git tags

Many projects sign their Git tags, but not their release tarballs. For a
`git-tag` source, kbuild does these steps (it needs no Git program):

1. It gets the tag, the commit and the trees from `git` with the Git
   protocol version 2 over HTTPS. It does not get the file contents.
2. It checks the OpenPGP signature of the tag with `key`.
3. It calculates the Git object ID of each file in the tarball. Each file
   must be the same as the file of the same path in the signed tree.

Some tarballs are not the same as the tree:

- `extra-files` are the files that the tarball adds or changes (for
  example generated files). The signature does not cover them, so keep
  this list short. Use the archive of the tag if possible.
- `missing-files` are the files of the tree that the tarball does not
  have (for example the files with `export-ignore` in `.gitattributes`).
- `subtrees` are the directories that come from another repository
  (`"path=repository"`). The commit is the submodule of the signed tree,
  or the `revision` of the Meson wrap file `path.wrap` in the signed tree.
  kbuild compares the files with the tree of that commit.

## Sources without signatures

These upstream projects do not sign their releases, or sign them only with
a weak key. Their recipes have only a pinned SHA-256 value:

- toybox, dash, dinit, popt, json-c, libaio, argp-standalone, mtdev,
  libudev-zero, seatd, libxkbcommon, libffi, utf8proc, the DejaVu fonts,
  the CNI plugins, cri-tools, acpica, nasm, musl-fts, musl-obstack, lz4,
  samurai, pkgconf, pango, glslang, the SPIR-V and Vulkan SDK
  repositories, SPIRV-LLVM-Translator, libcap-ng, EDK II and its
  submodules (upstream publishes no signatures, or only unsigned or
  lightweight Git tags)
- libevdev and libinput (upstream signs with a DSA-1024 key; kbuild does
  not accept keys that weak)
- mako and PyYAML (PyPI has no attestations for these releases), and
  setuptools (only the Git tag is signed, not the wheel)
- PipeWire, pipewire-media-session, libepoxy, GTK 3, gdk-pixbuf and
  cbindgen (the tags are not signed; GNOME publishes the SHA-256 values of
  GTK and gdk-pixbuf, crates.io the value of cbindgen)
- SQLite (in python3-host; the SHA3-256 value agrees with sqlite.org)
- the Mozilla CA bundle from curl.se (served with HTTPS only)
- the Microsoft Secure Boot certificates (served with HTTPS only)

`./klos outdated` shows new releases. Look again for signatures when you
update a recipe.

## The trust chain

1. The host verifies the seed (the image of the KLOS builder) with the
   SHA-256 value in `builder/seed.conf`.
2. kbuild verifies each source (SHA-256 and the signature, if available).
   kbuild checks OpenPGP, minisign and Sigstore signatures and signed Git
   tags itself (Go); it needs no gpgv, minisign, cosign or git program.
3. kbuild signs each update bundle with the KLOS key (Ed25519).
4. The installed system verifies the update bundles with its public key,
   and dm-verity verifies each block of the OS image when it reads it.

The first seed came from a build that started with Alpine Linux. A second
build that started with Arch Linux (GCC and glibc) gave the same images,
packages and toolchain (diverse double compiling). Since then, only KLOS
builds KLOS.
