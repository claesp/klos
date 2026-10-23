// SPDX-License-Identifier: BSD-2-Clause

package main

// "klos run-env FILE... -- PROGRAM ARG..." starts a program with arguments
// from environment files, without a shell. Each file has KEY="VALUE" lines
// (the format that kubeadm writes). A file that does not exist is empty.
// An argument "@KEY@" becomes the words of VALUE (or nothing).

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func readEnvFile(path string, vars map[string]string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		vars[strings.TrimSpace(k)] = v
	}
	return nil
}

func cmdRunEnv(args []string) error {
	vars := map[string]string{}
	i := 0
	for ; i < len(args) && args[i] != "--"; i++ {
		if err := readEnvFile(args[i], vars); err != nil {
			return err
		}
	}
	if i+1 >= len(args) {
		return errors.New("usage: klos run-env FILE... -- PROGRAM [ARG...]")
	}
	var argv []string
	for _, a := range args[i+1:] {
		if len(a) > 2 && a[0] == '@' && a[len(a)-1] == '@' {
			argv = append(argv, strings.Fields(vars[a[1:len(a)-1]])...)
			continue
		}
		argv = append(argv, a)
	}
	prog, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(prog, argv, os.Environ())
}
