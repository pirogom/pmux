package git

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// GetBranches lists the local branches and marks the current one.
func GetBranches(workDir string) ([]GitBranch, error) {
	bundle, err := openRepo(workDir)
	if err != nil {
		return nil, err
	}

	cur := currentBranch(bundle.repo)
	cfg, err := bundle.repo.Config()
	if err != nil {
		return nil, err
	}

	iter, err := bundle.repo.References()
	if err != nil {
		return nil, err
	}

	var branches []GitBranch
	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.SymbolicReference || !ref.Name().IsBranch() {
			return nil
		}
		name := ref.Name().Short()
		b := GitBranch{Name: name, Current: name == cur}
		if bc, ok := cfg.Branches[name]; ok && bc.Remote != "" {
			b.Upstream = bc.Remote + "/" + bc.Merge.Short()
		}
		branches = append(branches, b)
		return nil
	})
	return branches, nil
}

// Checkout switches the working tree to the given local branch.
func Checkout(workDir, branch string) GitOpResult {
	if branch == "" {
		return opErr(fmt.Errorf("branch name is empty"))
	}
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	if err := bundle.worktree.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branch),
	}); err != nil {
		return opErr(fmt.Errorf("checkout failed: %w", err))
	}
	return opOK("Switched to branch " + branch)
}
