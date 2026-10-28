package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type commit struct{ sha, tree, authorName, authorEmail, authorDate, committerName, committerEmail, committerDate, message string }
type plan struct {
	name, base, tip string
	commits         []string
}

func git(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func get(dir string, args ...string) string {
	s, e := git(dir, nil, args...)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	return s
}
func meta(dir, sha string) (commit, error) {
	cmd := exec.Command("git", "cat-file", "commit", sha)
	cmd.Dir = dir
	raw, err := cmd.Output()
	if err != nil {
		return commit{}, err
	}
	parts := bytes.SplitN(raw, []byte("\n\n"), 2)
	if len(parts) != 2 {
		return commit{}, errors.New("invalid commit object")
	}
	c := commit{sha: sha, message: string(parts[1])}
	for _, line := range strings.Split(string(parts[0]), "\n") {
		if strings.HasPrefix(line, "tree ") {
			c.tree = strings.TrimPrefix(line, "tree ")
		}
		if strings.HasPrefix(line, "author ") {
			c.authorName, c.authorEmail, c.authorDate, err = parseIdentity(strings.TrimPrefix(line, "author "))
			if err != nil {
				return commit{}, err
			}
		}
		if strings.HasPrefix(line, "committer ") {
			c.committerName, c.committerEmail, c.committerDate, err = parseIdentity(strings.TrimPrefix(line, "committer "))
			if err != nil {
				return commit{}, err
			}
		}
	}
	if c.tree == "" || c.authorDate == "" || c.committerDate == "" {
		return commit{}, errors.New("missing commit metadata")
	}
	return c, nil
}
func parseIdentity(v string) (string, string, string, error) {
	end := strings.LastIndex(v, "> ")
	if end < 0 {
		return "", "", "", errors.New("invalid commit identity")
	}
	start := strings.LastIndex(v[:end], " <")
	if start < 0 {
		return "", "", "", errors.New("invalid commit email")
	}
	return v[:start], v[start+2 : end], v[end+2:], nil
}
func ancestor(dir, base, tip string) bool {
	_, e := git(dir, nil, "merge-base", "--is-ancestor", base, tip)
	return e == nil
}
func revs(dir, base, tip string) ([]string, error) {
	s, e := git(dir, nil, "rev-list", "--reverse", "--ancestry-path", base+".."+tip)
	if e != nil {
		return nil, e
	}
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}
func main() {
	apply := flag.Bool("apply", false, "rewrite recovered branches after dry-run")
	anchorOld := flag.String("old-anchor", "8fe421c", "old commit corresponding to current main tip")
	mainRef := flag.String("main", "main", "new main branch")
	targetDir := flag.String("target-dir", ".", "path to the target local Git repository")
	flag.Parse()
	if flag.NArg() != 0 {
		fatal("unexpected positional arguments; use -target-dir to specify the repository path")
	}
	repo := get(*targetDir, "rev-parse", "--show-toplevel")
	if get(repo, "status", "--porcelain") != "" {
		fatal("working tree must be clean (including untracked files)")
	}
	if get(repo, "rev-parse", "--is-shallow-repository") == "true" {
		fatal("shallow repositories are unsupported")
	}
	names := []string{"recovered/pr-1", "recovered/pr-2", "recovered/pr-3"}
	mainTip := get(repo, "rev-parse", "--verify", "refs/heads/"+*mainRef+"^{commit}")
	oldAnchor := get(repo, "rev-parse", "--verify", *anchorOld+"^{commit}")
	oldMeta, e := meta(repo, oldAnchor)
	if e != nil {
		fatal(e.Error())
	}
	mainMeta, e := meta(repo, mainTip)
	if e != nil {
		fatal(e.Error())
	}
	if oldMeta.message != mainMeta.message {
		fatal("old anchor and main tip have different commit messages; verify the intended correspondence")
	}
	var tips [3]string
	for i, n := range names {
		tips[i] = get(repo, "rev-parse", "--verify", "refs/heads/"+n+"^{commit}")
	}
	plans := make([]plan, 3)
	for i, n := range names {
		base := oldAnchor
		if i > 0 {
			base = get(repo, "merge-base", tips[i-1], tips[i])
		}
		if !ancestor(repo, base, tips[i]) {
			fatal("old base is not an ancestor of " + n)
		}
		if i == 0 && !ancestor(repo, oldAnchor, tips[0]) {
			fatal("old anchor is not an ancestor of PR-1")
		}
		ids, e := revs(repo, base, tips[i])
		if e != nil {
			fatal(e.Error())
		}
		for _, id := range ids {
			parents := strings.Fields(get(repo, "rev-list", "--parents", "-n", "1", id))
			if len(parents) != 2 {
				fatal("merge or root commit in replay range: " + id + " (unsupported to avoid changing topology silently)")
			}
		}
		plans[i] = plan{name: n, base: base, tip: tips[i], commits: ids}
	}
	fmt.Printf("Current main: %s\nOld anchor:   %s\n", mainTip, oldAnchor)
	for _, p := range plans {
		fmt.Printf("%s: old base %s; %d commits to replay\n", p.name, p.base, len(p.commits))
	}
	if !*apply {
		fmt.Println("DRY RUN ONLY. No changes. Run with -apply to execute.")
		return
	}
	// Isolated worktree: never use the user's index or checkout during replay.
	temp, e := os.MkdirTemp("", "git-recovered-rebuild-*")
	if e != nil {
		fatal(e.Error())
	}
	if e = os.Remove(temp); e != nil {
		fatal(e.Error())
	}
	if _, e = git(repo, nil, "worktree", "add", "--detach", temp, mainTip); e != nil {
		fatal(e.Error())
	}
	defer func() { _, _ = git(repo, nil, "worktree", "remove", "--force", temp); _ = os.RemoveAll(temp) }()
	target := mainTip
	newTips := make([]string, 3)
	for i, p := range plans {
		fmt.Printf("\nReplaying %s onto %s\n", p.name, target)
		if _, e = git(temp, nil, "reset", "--hard", target); e != nil {
			fatal(e.Error())
		}
		for _, sha := range p.commits {
			original, e := meta(repo, sha)
			if e != nil {
				fatal(e.Error())
			}
			// Preserve the original patch by using cherry-pick's merge machinery, but not its commit creator.
			if _, e = git(temp, nil, "-c", "rerere.enabled=false", "cherry-pick", "--no-commit", sha); e != nil {
				fatal(fmt.Sprintf("Conflict or empty cherry-pick at %s (%s). Original branches remain unchanged.\n%v", p.name, sha, e))
			}
			tree, e := git(temp, nil, "write-tree")
			if e != nil {
				fatal(e.Error())
			}
			env := []string{"GIT_AUTHOR_NAME=" + original.authorName, "GIT_AUTHOR_EMAIL=" + original.authorEmail, "GIT_AUTHOR_DATE=" + original.authorDate, "GIT_COMMITTER_NAME=" + original.committerName, "GIT_COMMITTER_EMAIL=" + original.committerEmail, "GIT_COMMITTER_DATE=" + original.committerDate}
			cmd := exec.Command("git", "commit-tree", tree, "-p", target)
			cmd.Dir = temp
			cmd.Env = append(os.Environ(), env...)
			cmd.Stdin = bytes.NewBufferString(original.message)
			out, err := cmd.CombinedOutput()
			if err != nil {
				fatal(fmt.Sprintf("commit-tree %s: %v: %s", sha, err, out))
			}
			next := strings.TrimSpace(string(out))
			if _, e = git(temp, nil, "reset", "--hard", next); e != nil {
				fatal(e.Error())
			}
			fmt.Printf("  %s -> %s  %s\n", sha[:12], next[:12], strings.SplitN(original.message, "\n", 2)[0])
			target = next
		}
		newTips[i] = target
	}
	// Only update the branches after ALL replays have succeeded. CAS prevents accidental overwrites.
	// Keep backups in refs/backup to make undo possible even after reflog expiry.
	for _, p := range plans {
		backup := "refs/backup/recovered-rebuild/" + strings.TrimPrefix(p.name, "recovered/")
		if _, e = git(repo, nil, "show-ref", "--verify", "--quiet", backup); e == nil {
			fatal("backup ref already exists: " + backup + "; refusing overwrite")
		}
		if _, e = git(repo, nil, "update-ref", backup, p.tip, "0000000000000000000000000000000000000000"); e != nil {
			fatal(e.Error())
		}
		fmt.Printf("Backup: %s -> %s\n", backup, p.tip)
	}
	// Atomic transaction of the three local branch updates.
	var in strings.Builder
	in.WriteString("start\n")
	for i, p := range plans {
		fmt.Fprintf(&in, "update refs/heads/%s %s %s\n", p.name, newTips[i], p.tip)
	}
	in.WriteString("prepare\ncommit\n")
	cmd := exec.Command("git", "update-ref", "--stdin")
	cmd.Dir = repo
	cmd.Stdin = strings.NewReader(in.String())
	b, err := cmd.CombinedOutput()
	if err != nil {
		fatal(fmt.Sprintf("branch transaction failed (backups retained): %v: %s", err, b))
	}
	fmt.Println("\nSUCCESS: all three branches updated atomically; main unchanged.")
	fmt.Println("Check with: git log --graph --oneline --all --decorate")
	fmt.Println("Backups are stored under refs/backup/recovered-rebuild/")
}
func fatal(s string) { fmt.Fprintln(os.Stderr, "ERROR:", s); os.Exit(1) }
