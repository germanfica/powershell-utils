package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

type pr struct {
	number int
	sha    string
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func die(err error) { fmt.Fprintln(os.Stderr, "ERROR:", err); os.Exit(1) }
func namesFile(path string) (map[int]string, error) {
	names := map[int]string{}
	if path == "" {
		return names, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for line := 1; s.Scan(); line++ {
		t := strings.TrimSpace(s.Text())
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		pair := strings.SplitN(t, "=", 2)
		if len(pair) != 2 {
			return nil, fmt.Errorf("%s:%d: esperado NUMERO=nombre/rama", path, line)
		}
		n, err := strconv.Atoi(strings.TrimSpace(pair[0]))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("%s:%d: numero de PR invalido", path, line)
		}
		name := strings.TrimSpace(pair[1])
		if _, err = git("check-ref-format", "--branch", name); err != nil {
			return nil, fmt.Errorf("%s:%d: rama invalida %q", path, line, name)
		}
		names[n] = name
	}
	return names, s.Err()
}
func main() {
	repo := flag.String("repo", "", "OWNER/REPO (opcional; solo informativo)")
	source := flag.String("source", "origin", "Remote Git o URL del repositorio que contiene los PR")
	namesPath := flag.String("names", "", "Archivo opcional: numeroPR=nombre/original-de-rama")
	limit := flag.Int("limit", 1000, "Maximo de PR a procesar (ordenados por numero)")
	apply := flag.Bool("apply", false, "Crear ramas locales; por defecto simular")
	flag.Parse()
	if *limit < 1 {
		die(fmt.Errorf("-limit debe ser positivo"))
	}
	if _, err := git("rev-parse", "--show-toplevel"); err != nil {
		die(err)
	}
	names, err := namesFile(*namesPath)
	if err != nil {
		die(err)
	}
	out, err := git("ls-remote", *source, "refs/pull/*/head")
	if err != nil {
		die(err)
	}
	var prs []pr
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		ref := fields[1]
		parts := strings.Split(ref, "/")
		if len(parts) != 4 || parts[0] != "refs" || parts[1] != "pull" || parts[3] != "head" {
			continue
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil || n < 1 {
			continue
		}
		prs = append(prs, pr{n, fields[0]})
	}
	sort.Slice(prs, func(i, j int) bool { return prs[i].number < prs[j].number })
	if len(prs) > *limit {
		prs = prs[:*limit]
	}
	fmt.Printf("Repositorio: %s | Fuente: %s | PR encontrados: %d | apply=%v\n", *repo, *source, len(prs), *apply)
	if len(prs) == 0 {
		fmt.Println("No hay referencias refs/pull/*/head accesibles en esa fuente.")
		return
	}
	failures := 0
	for _, p := range prs {
		name, ok := names[p.number]
		if !ok {
			name = fmt.Sprintf("recovered/pr-%d", p.number)
		}
		ref := "refs/heads/" + name
		// Verify that no existing local ref is changed or overwritten.
		_, err := git("show-ref", "--verify", "--quiet", ref)
		if err == nil {
			fmt.Printf("PR #%d -> %s: ya existe, omitida\n", p.number, name)
			continue
		}
		fmt.Printf("PR #%d -> %s (%s)\n", p.number, name, p.sha)
		if !*apply {
			continue
		}
		// Fetch the original commit and ancestors; do not check out or rewrite commits.
		_, err = git("fetch", "--no-tags", *source, fmt.Sprintf("refs/pull/%d/head", p.number))
		if err != nil {
			fmt.Printf("  ERROR fetch: %v\n", err)
			failures++
			continue
		}
		sha, err := git("rev-parse", "--verify", "FETCH_HEAD^{commit}")
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			failures++
			continue
		}
		if sha != p.sha {
			fmt.Printf("  ERROR: referencia remota cambio durante la ejecucion (%s != %s); omitida\n", sha, p.sha)
			failures++
			continue
		}
		// Atomic creation with zero OID: fails if branch already exists.
		_, err = git("update-ref", ref, sha, strings.Repeat("0", 40))
		if err != nil {
			fmt.Printf("  ERROR al crear: %v\n", err)
			failures++
			continue
		}
		dates, _ := git("show", "-s", "--format=author=%aI committer=%cI", sha)
		fmt.Printf("  Creada localmente; %s\n", dates)
	}
	if !*apply {
		fmt.Println("Simulacion: no se modifico ningun ref local. Para crear ramas, agrega -apply.")
	}
	if failures > 0 {
		os.Exit(1)
	}
}
