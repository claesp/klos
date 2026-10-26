// SPDX-License-Identifier: BSD-2-Clause

package main

// The SELinux file contexts of KLOS (docs/selinux.md). kbuild uses them
// to label the OS image, and klos uses them to label /var, /run and /dev.
// The same file is in tools/kbuild and in src/klos ("./klos check" makes
// sure that the two copies are the same).
//
// Each line of a file_contexts file has this format:
//
//	REGEX [TYPE] CONTEXT
//
// REGEX must match the full path (Go regular expression syntax). TYPE is
// one of -- (regular file), -d, -l, -c, -b, -p or -s. Without TYPE, the
// line applies to all file types. CONTEXT is an SELinux context, or
// <<none>> for "do not label". The last line that matches a file applies.
// Thus put the general lines first and the specific lines after them.

import (
	"bufio"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

type fcRule struct {
	re      *regexp.Regexp
	kind    byte // 0 (all types), '-', 'd', 'l', 'c', 'b', 'p' or 's'
	context string
}

type fileContexts struct {
	rules []fcRule
}

// fcNone is the context of a file that must not get a label.
const fcNone = "<<none>>"

func parseFileContexts(text string) (*fileContexts, error) {
	fc := &fileContexts{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		r := fcRule{}
		switch len(f) {
		case 2:
		case 3:
			if len(f[1]) != 2 || f[1][0] != '-' || !strings.ContainsRune("-dlcbps", rune(f[1][1])) {
				return nil, fmt.Errorf("file_contexts line %d: bad file type %q", n, f[1])
			}
			r.kind = f[1][1]
		default:
			return nil, fmt.Errorf("file_contexts line %d: want REGEX [TYPE] CONTEXT", n)
		}
		re, err := regexp.Compile("^(?:" + f[0] + ")$")
		if err != nil {
			return nil, fmt.Errorf("file_contexts line %d: %w", n, err)
		}
		r.re, r.context = re, f[len(f)-1]
		if r.context != fcNone && strings.Count(r.context, ":") < 3 {
			return nil, fmt.Errorf("file_contexts line %d: bad context %q", n, r.context)
		}
		fc.rules = append(fc.rules, r)
	}
	return fc, sc.Err()
}

// fileKind returns the file_contexts type letter of a file mode.
func fileKind(m fs.FileMode) byte {
	switch {
	case m.IsDir():
		return 'd'
	case m&fs.ModeSymlink != 0:
		return 'l'
	case m&fs.ModeCharDevice != 0:
		return 'c'
	case m&fs.ModeDevice != 0:
		return 'b'
	case m&fs.ModeNamedPipe != 0:
		return 'p'
	case m&fs.ModeSocket != 0:
		return 's'
	}
	return '-'
}

// lookup returns the context of a path, or "" if no line matches or the
// line has <<none>>.
func (fc *fileContexts) lookup(path string, mode fs.FileMode) string {
	k := fileKind(mode)
	for i := len(fc.rules) - 1; i >= 0; i-- {
		r := fc.rules[i]
		if (r.kind == 0 || r.kind == k) && r.re.MatchString(path) {
			if r.context == fcNone {
				return ""
			}
			return r.context
		}
	}
	return ""
}

// types returns the SELinux types that the contexts use.
func (fc *fileContexts) types() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range fc.rules {
		if r.context == fcNone {
			continue
		}
		t := strings.Split(r.context, ":")[2]
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
