// SPDX-License-Identifier: BSD-2-Clause

package main

// Merge of the system accounts after an update. The image has the system
// users and groups in /etc/passwd, /etc/group and /etc/shadow. When the
// /etc overlay upper layer has its own copy of a file (for example after
// the installer added a user), that copy hides the file of the image. Then
// a new version cannot add a system user. Before /init mounts the overlay,
// mergeAccounts adds the missing system entries to the upper layer:
//
//   - A user or group that is in the image, but not in the upper file, is
//     added. If its ID is used by a different name, it is not added and
//     init shows a warning.
//   - A user that was added gets the group memberships from the image.
//   - A user that was added gets the shadow entry from the image (a locked
//     password).
//
// The merge does not remove or change entries. A user that a new version
// does not have stays in the files. Read docs/filesystem.md.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// accountFile is the lines of a passwd(5), group(5) or shadow(5) file.
type accountFile struct {
	lines []string
}

func readAccountFile(path string) (*accountFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	a := &accountFile{}
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			a.lines = append(a.lines, l)
		}
	}
	return a, nil
}

func (a *accountFile) String() string {
	return strings.Join(a.lines, "\n") + "\n"
}

// find returns the index of the entry with the name, or -1.
func (a *accountFile) find(name string) int {
	for i, l := range a.lines {
		if f := strings.SplitN(l, ":", 2); f[0] == name {
			return i
		}
	}
	return -1
}

// idUser returns the name of the entry with the ID in field 2, or "".
func (a *accountFile) idUser(id string) string {
	for _, l := range a.lines {
		f := strings.Split(l, ":")
		if len(f) > 2 && f[2] == id {
			return f[0]
		}
	}
	return ""
}

// mergeResult is the change that mergeAccountFiles made.
type mergeResult struct {
	users, groups []string // the added names
	warnings      []string
	changed       map[string]bool // "passwd", "group", "shadow"
}

// mergeAccountFiles adds the system entries of img to local. A nil local
// file means that the upper layer does not have that file; then the file
// of the image is in use and it needs no merge.
func mergeAccountFiles(img, local map[string]*accountFile) mergeResult {
	r := mergeResult{changed: map[string]bool{}}
	added := map[string]bool{}
	if ip, lp := img["passwd"], local["passwd"]; ip != nil && lp != nil {
		for _, l := range ip.lines {
			f := strings.Split(l, ":")
			if len(f) < 7 || lp.find(f[0]) >= 0 {
				continue
			}
			if other := lp.idUser(f[2]); other != "" {
				r.warnings = append(r.warnings, fmt.Sprintf("system user %s: UID %s is used by %s", f[0], f[2], other))
				continue
			}
			lp.lines = append(lp.lines, l)
			added[f[0]] = true
			r.users = append(r.users, f[0])
			r.changed["passwd"] = true
		}
	}
	if ig, lg := img["group"], local["group"]; ig != nil && lg != nil {
		for _, l := range ig.lines {
			f := strings.Split(l, ":")
			if len(f) != 4 {
				continue
			}
			i := lg.find(f[0])
			if i < 0 {
				if other := lg.idUser(f[2]); other != "" {
					r.warnings = append(r.warnings, fmt.Sprintf("system group %s: GID %s is used by %s", f[0], f[2], other))
					continue
				}
				// Only the members that the merge added: the
				// administrator can have removed the other members.
				f[3] = strings.Join(filterNames(strings.Split(f[3], ","), added), ",")
				lg.lines = append(lg.lines, strings.Join(f, ":"))
				r.groups = append(r.groups, f[0])
				r.changed["group"] = true
				continue
			}
			lf := strings.Split(lg.lines[i], ":")
			if len(lf) != 4 {
				continue
			}
			members := strings.FieldsFunc(lf[3], func(c rune) bool { return c == ',' })
			have := map[string]bool{}
			for _, m := range members {
				have[m] = true
			}
			for _, m := range filterNames(strings.Split(f[3], ","), added) {
				if !have[m] {
					members = append(members, m)
					r.changed["group"] = true
				}
			}
			lf[3] = strings.Join(members, ",")
			lg.lines[i] = strings.Join(lf, ":")
		}
	}
	if is, ls := img["shadow"], local["shadow"]; is != nil && ls != nil {
		for _, u := range r.users {
			if ls.find(u) >= 0 {
				continue
			}
			if i := is.find(u); i >= 0 {
				ls.lines = append(ls.lines, is.lines[i])
			} else {
				ls.lines = append(ls.lines, u+":!:0::::::")
			}
			r.changed["shadow"] = true
		}
	}
	return r
}

// filterNames returns the names that are in keep, in their order.
func filterNames(names []string, keep map[string]bool) []string {
	var out []string
	for _, n := range names {
		if n != "" && keep[n] {
			out = append(out, n)
		}
	}
	return out
}

var accountModes = map[string]os.FileMode{"passwd": 0o644, "group": 0o644, "shadow": 0o600}

// mergeAccounts merges the account files of the image /etc (imgEtc) into
// the overlay upper layer (upper).
func mergeAccounts(imgEtc, upper string) error {
	img, local := map[string]*accountFile{}, map[string]*accountFile{}
	for name := range accountModes {
		a, err := readAccountFile(filepath.Join(imgEtc, name))
		if err != nil {
			return err
		}
		img[name] = a
		l, err := readAccountFile(filepath.Join(upper, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		local[name] = l
	}
	r := mergeAccountFiles(img, local)
	for _, w := range r.warnings {
		ilog("warning: %s; it is not added", w)
	}
	for name := range r.changed {
		p := filepath.Join(upper, name)
		tmp := p + ".klos-new"
		if err := os.WriteFile(tmp, []byte(local[name].String()), accountModes[name]); err != nil {
			return err
		}
		if err := os.Rename(tmp, p); err != nil {
			return err
		}
	}
	if len(r.users) > 0 || len(r.groups) > 0 {
		ilog("added system accounts: users %v, groups %v", r.users, r.groups)
	}
	return nil
}
