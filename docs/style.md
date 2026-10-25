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

- Go: follow the usual Go patterns (the project uses the golang-patterns
  rules):
  - Use `gofmt`. `go vet` must not show problems.
  - Do not ignore an error without a reason. Write `_ =` and a comment when
    an error has no effect (for example a best-effort cleanup). Use
    `closeQuietly` for files that the program only read.
  - Wrap errors with context: `fmt.Errorf("enroll %s: %w", name, err)`.
  - Use sentinel errors (`errNoToken`) and check them with `errors.Is`.
  - Do not use naked returns or `panic` for errors.
  - Give network operations a context with a time limit.
  - Do not change package-level state during a command; give settings
    as parameters (for example `imageOptions`).
  - Subcommands parse their flags with `flag.ContinueOnError` and return
    the error.
  - Write unit tests for functions without side effects.
  - Run `./klos check` before a commit (gofmt, go vet and the tests).
- Go modules: `src/klos` uses the standard library and the vendored
  go-tpm module. `tools/kbuild` uses vendored modules (`vendor/`). Vendor
  each new module, and keep the list of modules short.
- Shell: use POSIX `sh` for the host script `klos`. Recipe scripts use bash.
- Each source file starts with `SPDX-License-Identifier: BSD-2-Clause`.
- Write comments with the same STE rules.

## Commits

- Write the first line in the imperative ("Add the desktop flavor").
- Write why the change is necessary in the body.
