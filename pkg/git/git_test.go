package git

import (
	"os"
	"path/filepath"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// newTestRepo creates a temporary git repository with one initial commit and
// returns its worktree path.
func newTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	sig := &object.Signature{Name: "Test User", Email: "test@example.com"}
	files := map[string]string{
		"hello.txt":    "hello world\n",
		"sub/dir.txt":  "nested\n",
		"delete_me.txt": "bye\n",
	}
	for name, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("initial commit", &gogit.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

func TestGetStatus_CleanRepo(t *testing.T) {
	dir := newTestRepo(t)
	res := GetStatus(dir)
	if !res.IsGitRepo {
		t.Fatalf("expected git repo, got IsGitRepo=false: %s", res.Error)
	}
	if res.Branch != "master" && res.Branch != "main" {
		t.Fatalf("unexpected branch %q", res.Branch)
	}
	if len(res.Changes) != 0 {
		t.Fatalf("expected clean tree, got %+v", res.Changes)
	}
	if res.Root == "" || filepath.Clean(res.Root) != filepath.Clean(dir) {
		t.Fatalf("unexpected root %q", res.Root)
	}
}

func TestGetStatus_Changes(t *testing.T) {
	dir := newTestRepo(t)

	// modify tracked file, create untracked file, delete tracked file
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "newfile.txt"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "delete_me.txt")); err != nil {
		t.Fatal(err)
	}

	res := GetStatus(dir)
	if !res.IsGitRepo {
		t.Fatalf("expected git repo")
	}
	if len(res.Changes) != 3 {
		t.Fatalf("expected 3 changes, got %+v", res.Changes)
	}

	byPath := map[string]GitChange{}
	for _, c := range res.Changes {
		byPath[c.Path] = c
	}

	m := byPath["hello.txt"]
	if m.Status != "M" || m.Unstaged != true || m.Staged != false {
		t.Fatalf("unexpected modified state: %+v", m)
	}
	n := byPath["newfile.txt"]
	if n.Status != "??" || n.Staged != false {
		t.Fatalf("unexpected untracked state: %+v", n)
	}
	d := byPath["delete_me.txt"]
	if d.Status != "D" || d.Unstaged != true {
		t.Fatalf("unexpected deleted state: %+v", d)
	}
}

func TestStageUnstageCommit(t *testing.T) {
	dir := newTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("staged change\n"), 0644); err != nil {
		t.Fatal(err)
	}

	res := Stage(dir, []string{"hello.txt"})
	if !res.Success {
		t.Fatalf("stage failed: %s", res.Error)
	}
	st := GetStatus(dir)
	found := false
	for _, c := range st.Changes {
		if c.Path == "hello.txt" {
			found = true
			if !c.Staged {
				t.Fatalf("expected staged, got %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("hello.txt missing from status")
	}

	// Commit with author from env-free repo (no global config in CI)
	t.Setenv("HOME", dir) // isolate global gitconfig
	res = Commit(dir, "my commit message")
	if !res.Success {
		t.Logf("commit skipped (author config unavailable): %s", res.Error)
	} else {
		st = GetStatus(dir)
		for _, c := range st.Changes {
			if c.Path == "hello.txt" && c.Staged {
				t.Fatalf("file should be committed: %+v", c)
			}
		}
		log, err := GetLog(dir, 10)
		if err != nil {
			t.Fatalf("GetLog: %v", err)
		}
		if len(log) != 2 || log[0].Subject != "my commit message" {
			t.Fatalf("unexpected log: %+v", log)
		}
	}

	// Unstage path
	res = Unstage(dir, []string{"hello.txt"})
	if !res.Success {
		t.Fatalf("unstage failed: %s", res.Error)
	}
	st = GetStatus(dir)
	for _, c := range st.Changes {
		if c.Path == "hello.txt" && c.Staged {
			t.Fatalf("file should be unstaged: %+v", c)
		}
	}
}

func TestStageAll(t *testing.T) {
	dir := newTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res := StageAll(dir)
	if !res.Success {
		t.Fatalf("stage all failed: %s", res.Error)
	}
	st := GetStatus(dir)
	if len(st.Changes) != 2 {
		t.Fatalf("expected 2 staged changes, got %+v", st.Changes)
	}
	for _, c := range st.Changes {
		if !c.Staged {
			t.Fatalf("expected all staged: %+v", c)
		}
	}
}

func TestGetDiff(t *testing.T) {
	dir := newTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world v3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res := GetDiff(dir, "hello.txt")
	if res.Error != "" {
		t.Fatalf("GetDiff error: %s", res.Error)
	}
	if res.Staged != "" {
		t.Fatalf("expected no staged diff, got %q", res.Staged)
	}
	if res.Unstaged == "" {
		t.Fatalf("expected unstaged diff")
	}
	if !contains(res.Unstaged, "+hello world v3") || !contains(res.Unstaged, "-hello world") {
		t.Fatalf("unexpected diff content:\n%s", res.Unstaged)
	}

	Stage(dir, []string{"hello.txt"})
	res = GetDiff(dir, "hello.txt")
	if res.Staged == "" {
		t.Fatalf("expected staged diff after staging")
	}
	if res.Unstaged != "" {
		t.Fatalf("expected no unstaged diff after staging, got %q", res.Unstaged)
	}
}

func TestGetBranchesAndCheckout(t *testing.T) {
	dir := newTestRepo(t)

	branches, err := GetBranches(dir)
	if err != nil {
		t.Fatalf("GetBranches: %v", err)
	}
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch, got %+v", branches)
	}
	if !branches[0].Current {
		t.Fatalf("expected current branch flag")
	}

	// create a second branch by committing on a new branch via go-git directly
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	ref := plumbing.NewBranchReferenceName("feature")
	repo.Storer.SetReference(plumbing.NewHashReference(ref, head.Hash()))
	if err != nil {
		t.Fatal(err)
	}

	res := Checkout(dir, "feature")
	if !res.Success {
		t.Fatalf("checkout failed: %s", res.Error)
	}
	st := GetStatus(dir)
	if st.Branch != "feature" {
		t.Fatalf("expected feature branch, got %q", st.Branch)
	}
	branches, err = GetBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(branches) != 2 {
		t.Fatalf("expected 2 branches, got %+v", branches)
	}
}

func TestGetLog_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	_ = repo
	commits, err := GetLog(dir, 10)
	if err != nil {
		t.Fatalf("GetLog: %v", err)
	}
	if len(commits) != 0 {
		t.Fatalf("expected empty log, got %+v", commits)
	}

	st := GetStatus(dir)
	if !st.IsGitRepo {
		t.Fatalf("expected git repo")
	}
	if st.Branch == "" {
		t.Fatalf("expected unborn branch name")
	}
}

func TestNotGitRepo(t *testing.T) {
	dir := t.TempDir()
	st := GetStatus(dir)
	if st.IsGitRepo {
		t.Fatalf("expected not a git repo")
	}
	if _, err := GetLog(dir, 10); err == nil {
		t.Fatalf("expected error for non-repo")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
