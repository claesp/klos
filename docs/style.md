# Style rules

## Documentation

All documentation uses Simplified Technical English (ASD-STE100). Use these
rules:

- Use one word for one meaning. For example, always use "check" for a
  check. Do not use "verify", "confirm" and "check" for the same action.
- Use the active voice: "kbuild makes the image", not "the image is made".
- Use simple tenses: the present, the simple past and the simple future.
- Write one instruction in one sentence. Use the imperative: "Type the
  passphrase."
- Write instructions with 20 words or fewer, and descriptions with 25
  words or fewer.
- Use a maximum of three nouns together.
- Use lists for steps and for three or more conditions.
- Write one topic in one paragraph, with a maximum of six sentences.
- Write "Caution:" before a step that can cause loss of data.

Keep all documentation in `docs/`, except `README.md`.

## Code

- Go: use `gofmt`. Use only the standard library in `src/klos`.
  `tools/kbuild` can use vendored modules (`vendor/`).
- Shell: use POSIX `sh` for the host script `klos`. Recipe scripts use bash.
- Each source file starts with `SPDX-License-Identifier: BSD-2-Clause`.
- Write comments with the same STE rules.

## Commits

- Write the first line in the imperative ("Add the desktop flavor").
- Write why the change is necessary in the body.
