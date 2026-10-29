// SPDX-License-Identifier: BSD-2-Clause

package main

// "kbuild outdated" finds new upstream releases of the recipes and known
// vulnerabilities (OSV.dev) in the pinned versions. Read docs/maintenance.md.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const upstreamTimeout = 90 * time.Second

// versionRe matches a release version: numbers with dots or underscores.
const versionRe = `([0-9]+(?:[._][0-9]+)*)`

// compareVersions compares two versions field by field (numbers compare as
// numbers). It returns -1, 0 or 1.
func compareVersions(a, b string) int {
	fa, fb := versionFields(a), versionFields(b)
	for i := 0; i < len(fa) || i < len(fb); i++ {
		var x, y int
		if i < len(fa) {
			x = fa[i]
		}
		if i < len(fb) {
			y = fb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// versionFields returns the numbers in a version ("10.6p1" is 10, 6, 1).
func versionFields(v string) []int {
	var out []int
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
		n, err := strconv.Atoi(f)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}

// sameSeries tells if v is in the release series of cur: the same first
// two fields (the same first field if cur has at most two fields).
func sameSeries(cur, v string) bool {
	fc, fv := versionFields(cur), versionFields(v)
	n := 2
	if len(fc) <= 2 {
		n = 1
	}
	if len(fc) < n || len(fv) < n {
		return false
	}
	for i := 0; i < n; i++ {
		if fc[i] != fv[i] {
			return false
		}
	}
	return true
}

// versionVariants are the forms of a version in file and tag names.
func versionVariants(v string) []string {
	out := []string{v}
	for _, sep := range []string{"_", "-"} {
		if w := strings.ReplaceAll(v, ".", sep); w != v {
			out = append(out, w)
		}
	}
	return out
}

func normalizeVersion(v string) string {
	return strings.NewReplacer("_", ".", "-", ".").Replace(v)
}

// patternFor makes a regular expression from a name that contains the
// version (a file name or a tag): the version becomes versionRe.
func patternFor(name, version string) (*regexp.Regexp, bool) {
	for _, v := range versionVariants(version) {
		if i := strings.Index(name, v); i >= 0 {
			re := "^" + regexp.QuoteMeta(name[:i]) + versionRe + regexp.QuoteMeta(name[i+len(v):]) + "$"
			return regexp.MustCompile(re), true
		}
	}
	return nil, false
}

// gitRepoFor returns the Git repository of a source URL on a known forge.
func gitRepoFor(u string) (string, bool) {
	p, err := url.Parse(u)
	if err != nil {
		return "", false
	}
	parts := strings.Split(strings.Trim(p.Path, "/"), "/")
	switch p.Host {
	case "github.com", "codeberg.org":
		if len(parts) >= 2 {
			return "https://" + p.Host + "/" + parts[0] + "/" + parts[1] + ".git", true
		}
	case "gitlab.freedesktop.org", "gitlab.com", "gitlab.gnome.org":
		for i, s := range parts {
			if s == "-" && i >= 2 {
				return "https://" + p.Host + "/" + strings.Join(parts[:i], "/") + ".git", true
			}
		}
	case "git.sr.ht":
		if len(parts) >= 2 {
			return "https://git.sr.ht/" + parts[0] + "/" + parts[1], true
		}
	case "git.kernel.org":
		for i, s := range parts {
			if strings.HasSuffix(s, ".git") {
				return "https://git.kernel.org/" + strings.Join(parts[:i+1], "/"), true
			}
		}
	}
	return "", false
}

var httpClient = &http.Client{Timeout: upstreamTimeout}

// httpGet gets a page (a second time after a network error).
func httpGet(ctx context.Context, u string) ([]byte, error) {
	// The GNU mirror redirector has no directory listings.
	u = strings.Replace(u, "://ftpmirror.gnu.org/", "://ftp.gnu.org/", 1)
	b, err := httpGetOnce(ctx, u)
	var he *httpError
	if err != nil && !errors.As(err, &he) && ctx.Err() == nil {
		b, err = httpGetOnce(ctx, u)
	}
	return b, err
}

type httpError struct {
	url, status string
}

func (e *httpError) Error() string { return fmt.Sprintf("GET %s: HTTP %s", e.url, e.status) }

func httpGetOnce(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "klos-kbuild/1")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{u, resp.Status}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func pktLine(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }

// gitTags lists the tags of a Git repository with the "ls-refs" command of
// the Git protocol version 2 over HTTP (no Git program is necessary).
func gitTags(ctx context.Context, repo string) ([]string, error) {
	body := pktLine("command=ls-refs\n") + "0001" + pktLine("ref-prefix refs/tags/\n") + "0000"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(repo, "/")+"/git-upload-pack",
		strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "git/2.50-klos-kbuild")
	req.Header.Set("Git-Protocol", "version=2")
	req.Header.Set("Content-Type", "application/x-git-upload-pack-request")
	req.Header.Set("Accept", "application/x-git-upload-pack-result")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %s", repo, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	tags, err := parseLsRefs(data)
	if err == nil && len(tags) > 0 {
		return tags, nil
	}
	// A server without protocol version 2 (for example an old cgit):
	// the version 0 reference advertisement lists all references.
	if data, err = httpGet(ctx, strings.TrimSuffix(repo, "/")+"/info/refs?service=git-upload-pack"); err != nil {
		return nil, err
	}
	return parseLsRefs(data)
}

// parseLsRefs reads the pkt-lines of an ls-refs answer ("<oid> <ref>
// [attributes]") and returns the tag names.
func parseLsRefs(data []byte) ([]string, error) {
	seen := map[string]bool{}
	var tags []string
	for len(data) >= 4 {
		n, err := strconv.ParseUint(string(data[:4]), 16, 16)
		if err != nil {
			return nil, fmt.Errorf("bad pkt-line %q", data[:4])
		}
		if n < 4 {
			data = data[4:]
			continue
		}
		if int(n) > len(data) {
			return nil, errors.New("short pkt-line")
		}
		f := strings.Fields(string(data[4:n]))
		data = data[n:]
		if len(f) >= 2 && strings.HasPrefix(f[1], "refs/tags/") {
			t := strings.TrimSuffix(strings.TrimPrefix(strings.SplitN(f[1], "\x00", 2)[0], "refs/tags/"), "^{}")
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	return tags, nil
}

var hrefRe = regexp.MustCompile(`href="([^"?#]+)"`)

// linkNames returns the base names of the link targets of an HTML page.
func linkNames(body []byte) []string {
	var out []string
	for _, m := range hrefRe.FindAllSubmatch(body, -1) {
		out = append(out, path.Base(strings.TrimSuffix(string(m[1]), "/")))
	}
	return out
}

// pageNames returns the link targets (base names) of an HTML directory
// listing and the words of a text page.
func pageNames(body []byte) []string {
	return append(linkNames(body), strings.FieldsFunc(string(body), func(r rune) bool {
		return r == '"' || r == '<' || r == '>' || r == ' ' || r == '\n' || r == '\t' || r == '/'
	})...)
}

// matchVersions returns the versions in names that re matches.
func matchVersions(names []string, re *regexp.Regexp, ignore *regexp.Regexp) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		m := re.FindStringSubmatch(n)
		if m == nil || len(m) < 2 {
			continue
		}
		v := normalizeVersion(m[1])
		if seen[v] || (ignore != nil && ignore.MatchString(v)) {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// releases finds the versions that upstream released.
func releases(ctx context.Context, r *Recipe) ([]string, error) {
	if len(r.Source) == 0 {
		return nil, errors.New("no sources")
	}
	var ignore *regexp.Regexp
	if r.Watch.Ignore != "" {
		var err error
		if ignore, err = regexp.Compile(r.Watch.Ignore); err != nil {
			return nil, fmt.Errorf("watch.ignore: %w", err)
		}
	}
	var custom *regexp.Regexp
	if r.Watch.Regex != "" {
		var err error
		if custom, err = regexp.Compile(r.Watch.Regex); err != nil {
			return nil, fmt.Errorf("watch.regex: %w", err)
		}
	}
	src := r.Source[0].URL
	watch := r.Watch.URL
	repo, isGit := gitRepoFor(src)
	if watch != "" {
		repo, isGit = watch, strings.HasSuffix(watch, ".git") || r.Watch.Git
	}
	if isGit {
		tags, err := gitTags(ctx, repo)
		if err != nil {
			return nil, err
		}
		re := custom
		if re == nil {
			// The tag of the current version gives the pattern.
			best := ""
			for _, t := range tags {
				if p, ok := patternFor(t, r.Version); ok && p.MatchString(t) &&
					(best == "" || len(t) < len(best)) {
					best, re = t, p
				}
			}
			if re == nil {
				return nil, fmt.Errorf("no tag of version %s in %s", r.Version, repo)
			}
		}
		return matchVersions(tags, re, ignore), nil
	}
	page := watch
	re := custom
	if page == "" {
		i := strings.LastIndex(src, "/")
		page = src[:i+1]
		var ok bool
		if re, ok = patternFor(src[i+1:], r.Version); !ok {
			return nil, fmt.Errorf("the file name of %s has no version %s", src, r.Version)
		}
		re = regexp.MustCompile(strings.TrimPrefix(re.String(), "^"))
	}
	if re == nil {
		return nil, errors.New("watch.url needs watch.regex")
	}
	pages := []string{page}
	// A directory for each release series (for example .../glib/2.90/):
	// look also in the newer series directories.
	series := path.Base(strings.TrimSuffix(page, "/"))
	if strings.HasPrefix(r.Version, series+".") && r.Watch.URL == "" {
		parent := page[:len(page)-len(series)-1]
		body, err := httpGet(ctx, parent)
		if err != nil {
			return nil, err
		}
		sre := regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)*)$`)
		for _, s := range matchVersions(linkNames(body), sre, ignore) {
			if compareVersions(s, series) > 0 {
				pages = append(pages, parent+s+"/")
			}
		}
	}
	var versions []string
	for _, p := range pages {
		body, err := httpGet(ctx, p)
		if err != nil {
			return nil, err
		}
		versions = append(versions, matchVersions(pageNames(body), re, ignore)...)
	}
	return versions, nil
}

// UpstreamState is the result of a release check of one recipe.
type UpstreamState struct {
	Name, Current, Series, Newest, Note string
}

func checkUpstream(ctx context.Context, r *Recipe) UpstreamState {
	st := UpstreamState{Name: r.Name, Current: r.Version}
	if len(r.Source) == 0 {
		return st // a KLOS package (no upstream)
	}
	if r.Watch.Disabled != "" {
		st.Note = "not checked: " + r.Watch.Disabled
		return st
	}
	vs, err := releases(ctx, r)
	if err != nil {
		st.Note = "error: " + err.Error()
		return st
	}
	if len(vs) == 0 {
		st.Note = "error: no versions found"
		return st
	}
	prerelease := regexp.MustCompile(`(?i)(rc|alpha|beta|pre|dev)`)
	for _, v := range vs {
		if prerelease.MatchString(v) {
			continue
		}
		if compareVersions(v, st.Current) > 0 {
			if st.Newest == "" || compareVersions(v, st.Newest) > 0 {
				st.Newest = v
			}
			if sameSeries(st.Current, v) && (st.Series == "" || compareVersions(v, st.Series) > 0) {
				st.Series = v
			}
		}
	}
	return st
}

// --- OSV.dev ---

// OSVQuery is one package version to look up.
type OSVQuery struct {
	Recipe    string `json:"-"`
	Ecosystem string `json:"-"`
	Name      string `json:"-"`
	Version   string `json:"version"`
	Package   struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
}

func newOSVQuery(recipe, eco, name, version string) OSVQuery {
	q := OSVQuery{Recipe: recipe, Ecosystem: eco, Name: name, Version: version}
	q.Package.Name, q.Package.Ecosystem = name, eco
	return q
}

// goModules reads the module list of a Go source tarball: the main module
// (with the recipe version) and vendor/modules.txt.
func goModules(r *Recipe, tarball string) ([]OSVQuery, error) {
	names, err := tarNames(tarball)
	if err != nil {
		return nil, err
	}
	gomod := shortestWithBase(names, "go.mod")
	modtxt := shortestWithSuffix(names, "vendor/modules.txt")
	var qs []OSVQuery
	if gomod != "" {
		b, err := tarFile(tarball, gomod)
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(string(b), "\n") {
			if m, ok := strings.CutPrefix(strings.TrimSpace(l), "module "); ok {
				qs = append(qs, newOSVQuery(r.Name, "Go", strings.Trim(m, `"`), r.Version))
				break
			}
		}
	}
	if modtxt != "" {
		b, err := tarFile(tarball, modtxt)
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(string(b), "\n") {
			f := strings.Fields(l)
			// "# module version" (a replaced module has "=> ...").
			if len(f) >= 3 && f[0] == "#" && strings.HasPrefix(f[2], "v") {
				v := strings.TrimSuffix(strings.TrimPrefix(f[2], "v"), "+incompatible")
				qs = append(qs, newOSVQuery(r.Name, "Go", f[1], v))
			}
		}
	}
	return qs, nil
}

// cargoCrates reads the crates.io packages of the Cargo.lock files at the
// top of a Rust source tarball.
func cargoCrates(r *Recipe, tarball string) ([]OSVQuery, error) {
	names, err := tarNames(tarball)
	if err != nil {
		return nil, err
	}
	lock := shortestWithBase(names, "Cargo.lock")
	if lock == "" {
		return nil, nil
	}
	b, err := tarFile(tarball, lock)
	if err != nil {
		return nil, err
	}
	var qs []OSVQuery
	name, version := "", ""
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		switch {
		case l == "[[package]]":
			name, version = "", ""
		case strings.HasPrefix(l, "name = "):
			name = strings.Trim(strings.TrimPrefix(l, "name = "), `"`)
		case strings.HasPrefix(l, "version = "):
			version = strings.Trim(strings.TrimPrefix(l, "version = "), `"`)
		case strings.HasPrefix(l, "source = ") && strings.Contains(l, "crates.io"):
			if name != "" && version != "" {
				qs = append(qs, newOSVQuery(r.Name, "crates.io", name, version))
			}
		}
	}
	return qs, sc.Err()
}

func tarNames(tarball string) ([]string, error) {
	out, err := exec.Command("tar", "-tf", tarball).Output()
	if err != nil {
		return nil, fmt.Errorf("tar -tf %s: %w", tarball, err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
}

func tarFile(tarball, name string) ([]byte, error) {
	out, err := exec.Command("tar", "-xOf", tarball, name).Output()
	if err != nil {
		return nil, fmt.Errorf("tar -xOf %s %s: %w", tarball, name, err)
	}
	return out, nil
}

func pathDepth(p string) int { return strings.Count(strings.Trim(p, "/"), "/") }

func shortestWithBase(names []string, base string) string {
	best := ""
	for _, n := range names {
		if path.Base(n) == base && (best == "" || pathDepth(n) < pathDepth(best)) {
			best = n
		}
	}
	return best
}

func shortestWithSuffix(names []string, suffix string) string {
	best := ""
	for _, n := range names {
		if (n == suffix || strings.HasSuffix(n, "/"+suffix)) && (best == "" || pathDepth(n) < pathDepth(best)) {
			best = n
		}
	}
	return best
}

var pypiRe = regexp.MustCompile(`^([A-Za-z0-9_.]+?)-([0-9][0-9A-Za-z.]*?)(?:-py[0-9].*\.whl|\.tar\.gz|\.zip)$`)

// osvQueries collects the package versions of all recipes for OSV.dev.
func osvQueries(rs []*Recipe) ([]OSVQuery, []string) {
	var qs []OSVQuery
	var notes []string
	for _, r := range rs {
		for _, e := range r.Watch.OSV {
			eco, name, ok := strings.Cut(e, ":")
			if !ok {
				notes = append(notes, fmt.Sprintf("%s: watch.osv %q is not ecosystem:name", r.Name, e))
				continue
			}
			qs = append(qs, newOSVQuery(r.Name, eco, name, r.Version))
		}
		if r.Name == "go" {
			qs = append(qs, newOSVQuery(r.Name, "Go", "stdlib", r.Version))
		}
		for _, s := range r.Source {
			if strings.Contains(s.URL, "files.pythonhosted.org") {
				if m := pypiRe.FindStringSubmatch(s.filename()); m != nil {
					qs = append(qs, newOSVQuery(r.Name, "PyPI", m[1], m[2]))
				}
			}
		}
		if len(r.Source) == 0 || !exists(r.Source[0].path()) {
			continue
		}
		var more []OSVQuery
		var err error
		switch {
		case contains(r.HostDepends, "go"):
			more, err = goModules(r, r.Source[0].path())
		case contains(r.HostDepends, "rust") || r.Name == "rust":
			more, err = cargoCrates(r, r.Source[0].path())
		}
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s: %v", r.Name, err))
		}
		qs = append(qs, more...)
	}
	return qs, notes
}

// OSVVuln is the part of an OSV record that the report shows.
type OSVVuln struct {
	ID       string   `json:"id"`
	Summary  string   `json:"summary"`
	Aliases  []string `json:"aliases"`
	Affected []struct {
		Package struct {
			Name string `json:"name"`
		} `json:"package"`
		Ranges []struct {
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

// fixedIn returns the versions that fix the vulnerability in a package.
func (v OSVVuln) fixedIn(pkg string) []string {
	var out []string
	for _, a := range v.Affected {
		if a.Package.Name != pkg {
			continue
		}
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				if f, ok := e["fixed"]; ok {
					out = append(out, f)
				}
			}
		}
	}
	return uniqSorted(out)
}

func osvPost(ctx context.Context, u string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer closeQuietly(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: HTTP %s", u, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// OSVFinding is a vulnerability in one package version of a recipe.
type OSVFinding struct {
	Query OSVQuery
	Vuln  OSVVuln
}

// queryOSV asks OSV.dev for the vulnerabilities of the package versions.
func queryOSV(ctx context.Context, qs []OSVQuery) ([]OSVFinding, error) {
	type batchResult struct {
		Results []struct {
			Vulns []struct {
				ID string `json:"id"`
			} `json:"vulns"`
		} `json:"results"`
	}
	var findings []OSVFinding
	details := map[string]OSVVuln{}
	const batch = 500
	for start := 0; start < len(qs); start += batch {
		part := qs[start:min(start+batch, len(qs))]
		var res batchResult
		if err := osvPost(ctx, "https://api.osv.dev/v1/querybatch", map[string]any{"queries": part}, &res); err != nil {
			return nil, err
		}
		if len(res.Results) != len(part) {
			return nil, fmt.Errorf("OSV: %d results for %d queries", len(res.Results), len(part))
		}
		for i, r := range res.Results {
			for _, v := range r.Vulns {
				d, ok := details[v.ID]
				if !ok {
					b, err := httpGet(ctx, "https://api.osv.dev/v1/vulns/"+url.PathEscape(v.ID))
					if err != nil {
						return nil, err
					}
					if err := json.Unmarshal(b, &d); err != nil {
						return nil, fmt.Errorf("OSV %s: %w", v.ID, err)
					}
					details[v.ID] = d
				}
				findings = append(findings, OSVFinding{Query: part[i], Vuln: d})
			}
		}
	}
	return findings, nil
}

// cmdOutdated shows the recipes with newer upstream releases and the known
// vulnerabilities. With -fail, it exits with status 1 if it finds a
// vulnerability (for the nightly check).
func cmdOutdated(args []string) {
	fs := flag.NewFlagSet("outdated", flag.ContinueOnError)
	fail := fs.Bool("fail", false, "exit with status 1 if a vulnerability is known")
	noOSV := fs.Bool("no-osv", false, "do not query OSV.dev")
	if err := fs.Parse(args); err != nil {
		die("usage: outdated [-fail] [-no-osv] [package...]")
	}
	var rs []*Recipe
	if fs.NArg() > 0 {
		for _, n := range fs.Args() {
			rs = append(rs, recipe(n))
		}
	} else {
		for _, r := range recipes() {
			rs = append(rs, r)
		}
		rs = append(rs, kernelRecipe())
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].Name < rs[j].Name })
	ctx := context.Background()

	states := make([]UpstreamState, len(rs))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				cctx, cancel := context.WithTimeout(ctx, 3*upstreamTimeout)
				states[i] = checkUpstream(cctx, rs[i])
				cancel()
			}
		}()
	}
	for i := range rs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	newer, errs := 0, 0
	fmt.Printf("%-24s %-14s %-14s %-14s %s\n", "PACKAGE", "PINNED", "SAME SERIES", "NEWEST", "NOTE")
	for _, st := range states {
		if st.Series == "" && st.Newest == "" && st.Note == "" {
			continue
		}
		if st.Newest != "" {
			newer++
		}
		if strings.HasPrefix(st.Note, "error:") {
			errs++
		}
		fmt.Printf("%-24s %-14s %-14s %-14s %s\n", st.Name, st.Current, dash(st.Series), dash(st.Newest), st.Note)
	}
	logf("outdated: %d of %d recipes have a newer release; %d could not be checked", newer, len(rs), errs)
	if *noOSV {
		return
	}
	qs, notes := osvQueries(rs)
	for _, n := range notes {
		logf("warning: %s", n)
	}
	findings, err := queryOSV(ctx, qs)
	if err != nil {
		die("OSV.dev: %v", err)
	}
	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Query.Recipe != b.Query.Recipe {
			return a.Query.Recipe < b.Query.Recipe
		}
		if a.Query.Name != b.Query.Name {
			return a.Query.Name < b.Query.Name
		}
		return a.Vuln.ID < b.Vuln.ID
	})
	if len(findings) > 0 {
		fmt.Println()
		fmt.Println("Known vulnerabilities (OSV.dev) in the pinned versions:")
	}
	// One vulnerability can have records in more than one database (for
	// example GO-... and GHSA-...): show it once for each package version.
	shown := map[string]bool{}
	count := 0
	for _, f := range findings {
		key := f.Query.Recipe + "\x00" + f.Query.Name + "\x00" + f.Query.Version + "\x00"
		dup := shown[key+f.Vuln.ID]
		for _, a := range f.Vuln.Aliases {
			dup = dup || shown[key+a]
		}
		shown[key+f.Vuln.ID] = true
		for _, a := range f.Vuln.Aliases {
			shown[key+a] = true
		}
		if dup {
			continue
		}
		count++
		alias := ""
		if len(f.Vuln.Aliases) > 0 {
			alias = " (" + strings.Join(f.Vuln.Aliases, ", ") + ")"
		}
		fixed := strings.Join(f.Vuln.fixedIn(f.Query.Name), ", ")
		fmt.Printf("%-16s %s %s: %s%s; fixed in %s: %s\n", f.Query.Recipe, f.Query.Name, f.Query.Version,
			f.Vuln.ID, alias, dash(fixed), f.Vuln.Summary)
	}
	logf("outdated: %d package versions checked with OSV.dev; %d known vulnerabilities", len(qs), count)
	logf("a vulnerability in a Go or Rust module applies only if the program uses the affected code")
	if *fail && count > 0 {
		os.Exit(1)
	}
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// kernelRecipe makes a recipe for the release check of the kernel.
func kernelRecipe() *Recipe {
	k := kernelDef()
	return &Recipe{Name: "linux", Version: k.Version, Source: k.Source}
}
