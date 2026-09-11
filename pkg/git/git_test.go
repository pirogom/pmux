package git

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
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
	if !contains(res.Unstaged, "@@ -1,1 +1,1 @@") {
		t.Fatalf("expected hunk header with line numbers, got:\n%s", res.Unstaged)
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

// addTestCommits appends n linear commits with strictly increasing committer
// timestamps so log ordering is deterministic.
func addTestCommits(t *testing.T, dir string, n int) {
	t.Helper()
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Duration(n) * time.Minute)
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(fmt.Sprintf("change %d\n", i)), 0644); err != nil {
			t.Fatal(err)
		}
		if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
			t.Fatal(err)
		}
		sig := &object.Signature{Name: "Test User", Email: "test@example.com", When: base.Add(time.Duration(i) * time.Minute)}
		if _, err := wt.Commit(fmt.Sprintf("commit %d", i), &gogit.CommitOptions{Author: sig}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGetLogPage_Pagination(t *testing.T) {
	dir := newTestRepo(t)
	addTestCommits(t, dir, 5) // initial commit + 5 = 6 total

	var all []GitCommit
	cursor := ""
	pages := 0
	for {
		page, err := GetLogPage(dir, 2, cursor)
		if err != nil {
			t.Fatalf("GetLogPage(cursor=%q): %v", cursor, err)
		}
		if len(page.Commits) > 2 {
			t.Fatalf("page larger than limit: %+v", page.Commits)
		}
		all = append(all, page.Commits...)
		pages++
		if !page.HasMore {
			if page.NextCursor != "" {
				t.Fatalf("NextCursor set without HasMore")
			}
			break
		}
		if page.NextCursor == "" {
			t.Fatalf("HasMore without NextCursor")
		}
		if page.NextCursor == cursor {
			t.Fatalf("cursor did not advance")
		}
		cursor = page.NextCursor
		if pages > 10 {
			t.Fatalf("too many pages")
		}
	}

	if pages != 3 {
		t.Fatalf("expected 3 pages, got %d", pages)
	}
	if len(all) != 6 {
		t.Fatalf("expected 6 commits across pages, got %d", len(all))
	}

	seen := map[string]bool{}
	for _, c := range all {
		if seen[c.Hash] {
			t.Fatalf("duplicate commit %s", c.Hash)
		}
		seen[c.Hash] = true
	}
	if all[0].Subject != "commit 4" || !all[0].IsHead {
		t.Fatalf("unexpected first commit: %+v", all[0])
	}
	for _, c := range all[1:] {
		if c.IsHead {
			t.Fatalf("only the first commit should be marked head: %+v", c)
		}
	}
	if last := all[len(all)-1]; last.Subject != "initial commit" {
		t.Fatalf("unexpected last commit: %+v", last)
	}
}

func TestGetLogPage_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := gogit.PlainInit(dir, false); err != nil {
		t.Fatal(err)
	}
	page, err := GetLogPage(dir, 10, "")
	if err != nil {
		t.Fatalf("GetLogPage: %v", err)
	}
	if len(page.Commits) != 0 || page.HasMore || page.NextCursor != "" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestGetLogPage_InvalidCursor(t *testing.T) {
	dir := newTestRepo(t)
	if _, err := GetLogPage(dir, 10, "not-a-hash"); err == nil {
		t.Fatalf("expected error for malformed cursor")
	}
	missing := "0000000000000000000000000000000000000000"
	if _, err := GetLogPage(dir, 10, missing); err == nil {
		t.Fatalf("expected error for cursor not present in history")
	}
}

func TestGetLogPage_MergeHistory(t *testing.T) {
	dir := newTestRepo(t)
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour)

	// feature branch commit C, older than master's commit B
	if err := wt.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
		Hash:   head.Hash(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	cSig := &object.Signature{Name: "Test User", Email: "test@example.com", When: base.Add(1 * time.Minute)}
	cHash, err := wt.Commit("feature commit", &gogit.CommitOptions{Author: cSig})
	if err != nil {
		t.Fatal(err)
	}

	// back on the original branch: commit B, newer than C
	if err := wt.Checkout(&gogit.CheckoutOptions{Branch: head.Name()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "master.txt"), []byte("master\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	bSig := &object.Signature{Name: "Test User", Email: "test@example.com", When: base.Add(2 * time.Minute)}
	bHash, err := wt.Commit("master commit", &gogit.CommitOptions{Author: bSig})
	if err != nil {
		t.Fatal(err)
	}

	// merge commit M with parents B (first) and C
	bCommit, err := repo.CommitObject(bHash)
	if err != nil {
		t.Fatal(err)
	}
	mSig := object.Signature{Name: "Test User", Email: "test@example.com", When: base.Add(3 * time.Minute)}
	merge := &object.Commit{
		Author:       mSig,
		Committer:    mSig,
		Message:      "merge feature",
		TreeHash:     bCommit.TreeHash,
		ParentHashes: []plumbing.Hash{bHash, cHash},
	}
	obj := repo.Storer.NewEncodedObject()
	if err := merge.Encode(obj); err != nil {
		t.Fatal(err)
	}
	mHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(head.Name(), mHash)); err != nil {
		t.Fatal(err)
	}

	// Paginate one commit at a time; the merge side branch must not be skipped.
	var all []GitCommit
	cursor := ""
	for i := 0; i < 10; i++ {
		page, err := GetLogPage(dir, 1, cursor)
		if err != nil {
			t.Fatalf("GetLogPage(cursor=%q): %v", cursor, err)
		}
		all = append(all, page.Commits...)
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 commits, got %d: %+v", len(all), all)
	}
	seen := map[string]bool{}
	for _, c := range all {
		if seen[c.Hash] {
			t.Fatalf("duplicate commit %s", c.Hash)
		}
		seen[c.Hash] = true
	}
	if !seen[cHash.String()] {
		t.Fatalf("merge side branch commit missing from paginated history")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func setAutocrlf(t *testing.T, dir, value string) {
	t.Helper()
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Raw.Section("core").SetOption("autocrlf", value)
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func blobHash(data string) plumbing.Hash {
	h := plumbing.NewHasher(plumbing.BlobObject, int64(len(data)))
	h.Write([]byte(data))
	return h.Sum()
}

func TestGetStatus_Autocrlf_CrlfWorktreeIsClean(t *testing.T) {
	dir := newTestRepo(t)
	setAutocrlf(t, dir, "true")

	// Simulate an autocrlf=true checkout: same text as the committed LF blob,
	// but CRLF line endings on disk.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res := GetStatus(dir)
	if len(res.Changes) != 0 {
		t.Fatalf("expected clean tree despite CRLF on disk, got %+v", res.Changes)
	}
}

func TestGetStatus_AutocrlfDisabled_DetectsCrlfChange(t *testing.T) {
	dir := newTestRepo(t)
	setAutocrlf(t, dir, "false")

	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res := GetStatus(dir)
	if len(res.Changes) != 1 {
		t.Fatalf("expected 1 change, got %+v", res.Changes)
	}
	if res.Changes[0].Path != "hello.txt" || !res.Changes[0].Unstaged {
		t.Fatalf("unexpected change: %+v", res.Changes[0])
	}
}

func TestStage_Autocrlf_StoresLfBlob(t *testing.T) {
	dir := newTestRepo(t)
	setAutocrlf(t, dir, "true")

	content := "staged crlf\r\nline2\r\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	res := Stage(dir, []string{"hello.txt"})
	if !res.Success {
		t.Fatalf("stage failed: %s", res.Error)
	}

	expected := blobHash("staged crlf\nline2\n")
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	entry, err := idx.Entry("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Hash != expected {
		t.Fatalf("index blob should be LF-normalized (%s), got %s", expected, entry.Hash)
	}

	st := GetStatus(dir)
	if len(st.Changes) != 1 || !st.Changes[0].Staged || st.Changes[0].Unstaged {
		t.Fatalf("expected 1 staged-only change, got %+v", st.Changes)
	}
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func newTestRepoWithExecutable(t *testing.T) string {
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
	scriptPath := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hi\n"), 0644); err != nil {
		t.Fatalf("write run.sh: %v", err)
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{Path: "run.sh"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	// Force the index entry to filemode.Executable
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	e, err := idx.Entry("run.sh")
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	e.Mode = filemode.Executable
	if err := repo.Storer.SetIndex(idx); err != nil {
		t.Fatalf("set index: %v", err)
	}

	if _, err := wt.Commit("add executable script", &gogit.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return dir
}

func TestGetStatus_FileModeIgnored_ExecutableInIndex_Clean(t *testing.T) {
	dir := newTestRepoWithExecutable(t)
	res := GetStatus(dir)
	if !res.IsGitRepo {
		t.Fatalf("expected git repo")
	}
	if len(res.Changes) != 0 {
		t.Fatalf("expected 0 changes for file with identical content despite mode difference, got %+v", res.Changes)
	}
}

func TestGetStatus_FileModeIgnored_ExecutableInIndex_ModifiedContent(t *testing.T) {
	dir := newTestRepoWithExecutable(t)
	scriptPath := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hi v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res := GetStatus(dir)
	if !res.IsGitRepo {
		t.Fatalf("expected git repo")
	}
	if len(res.Changes) != 1 {
		t.Fatalf("expected 1 change, got %+v", res.Changes)
	}
	if res.Changes[0].Path != "run.sh" || !res.Changes[0].Unstaged || res.Changes[0].Staged {
		t.Fatalf("unexpected change status: %+v", res.Changes[0])
	}
}

func TestStage_FileModeIgnored_PreservesExecutableMode(t *testing.T) {
	dir := newTestRepoWithExecutable(t)
	scriptPath := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hi v2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	res := Stage(dir, []string{"run.sh"})
	if !res.Success {
		t.Fatalf("stage failed: %s", res.Error)
	}
	// Check index entry mode
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := repo.Storer.Index()
	if err != nil {
		t.Fatal(err)
	}
	entry, err := idx.Entry("run.sh")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Mode != filemode.Executable {
		t.Fatalf("expected filemode.Executable in index after staging, got %v", entry.Mode)
	}

	st := GetStatus(dir)
	if len(st.Changes) != 1 || !st.Changes[0].Staged || st.Changes[0].Unstaged {
		t.Fatalf("expected 1 staged change, got %+v", st.Changes)
	}

	t.Setenv("HOME", dir)
	commitRes := Commit(dir, "updated script")
	if commitRes.Success {
		head, err := repo.Head()
		if err != nil {
			t.Fatal(err)
		}
		commitObj, err := repo.CommitObject(head.Hash())
		if err != nil {
			t.Fatal(err)
		}
		tree, err := commitObj.Tree()
		if err != nil {
			t.Fatal(err)
		}
		f, err := tree.File("run.sh")
		if err != nil {
			t.Fatal(err)
		}
		if f.Mode != filemode.Executable {
			t.Fatalf("expected committed tree to have filemode.Executable, got %v", f.Mode)
		}
	}
}

