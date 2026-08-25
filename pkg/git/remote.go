package git

import (
	"context"
	"errors"
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// Push pushes the current branch to its remote (git push).
func Push(workDir string) GitOpResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	remote := pickRemote(bundle.repo, "origin")
	if remote == nil {
		return opErr(errors.New("no remote configured for this repository"))
	}
	auth, authDesc := resolveAuth(remote)

	head, err := bundle.repo.Head()
	if err != nil {
		return opErr(fmt.Errorf("cannot push: %w", err))
	}
	if !head.Name().IsBranch() {
		return opErr(errors.New("cannot push from a detached HEAD"))
	}
	branch := head.Name().Short()

	// Build the refspec: use the configured upstream (branch.<name>.remote and
	// branch.<name>.merge) when available, otherwise push to the same branch
	// name on the remote.
	refSpecs := []gitconfig.RefSpec{gitconfig.RefSpec(plumbing.NewBranchReferenceName(branch) + ":" + plumbing.NewBranchReferenceName(branch))}
	cfg, _ := bundle.repo.Config()
	upstream := cfg.Branches[branch]
	if upstream != nil && upstream.Remote == remote.Config().Name && upstream.Merge != "" {
		refSpecs = []gitconfig.RefSpec{gitconfig.RefSpec(plumbing.NewBranchReferenceName(branch) + ":" + upstream.Merge)}
	}

	err = remote.PushContext(context.Background(), &gogit.PushOptions{
		RemoteName: remote.Config().Name,
		Auth:       auth,
		RefSpecs:   refSpecs,
		FollowTags: true,
	})
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return opOK("Everything up-to-date.")
		}
		return opFail(err, authDesc)
	}

	// Remember the upstream so subsequent pushes default to it.
	if upstream == nil || upstream.Remote == "" {
		if cfg.Branches == nil {
			cfg.Branches = map[string]*gitconfig.Branch{}
		}
		cfg.Branches[branch] = &gitconfig.Branch{
			Name:   branch,
			Remote: remote.Config().Name,
			Merge:  plumbing.NewBranchReferenceName(branch),
		}
		_ = bundle.repo.Storer.SetConfig(cfg)
	}

	res := opOK(fmt.Sprintf("Pushed %s to %s (%s).", branch, remote.Config().Name, authDesc))
	if authDesc == "" {
		res.Output = fmt.Sprintf("Pushed %s to %s.", branch, remote.Config().Name)
	}
	return res
}

// Pull fetches and merges changes from the remote into the current branch
// (git pull).
func Pull(workDir string) GitOpResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	remote := pickRemote(bundle.repo, "origin")
	if remote == nil {
		return opErr(errors.New("no remote configured for this repository"))
	}
	auth, authDesc := resolveAuth(remote)

	err = bundle.worktree.PullContext(context.Background(), &gogit.PullOptions{
		RemoteName: remote.Config().Name,
		Auth:       auth,
	})
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return opOK("Already up to date.")
		}
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return opErr(errors.New("remote repository is empty"))
		}
		return opFail(err, authDesc)
	}

	res := opOK("Pull completed successfully.")
	if authDesc != "" {
		res.Auth = authDesc
	}
	return res
}

// Fetch downloads objects and refs from the remote (git fetch).
func Fetch(workDir string) GitOpResult {
	bundle, err := openRepo(workDir)
	if err != nil {
		return opErr(err)
	}
	remote := pickRemote(bundle.repo, "origin")
	if remote == nil {
		return opErr(errors.New("no remote configured for this repository"))
	}
	auth, authDesc := resolveAuth(remote)

	err = remote.FetchContext(context.Background(), &gogit.FetchOptions{
		RemoteName: remote.Config().Name,
		Auth:       auth,
		Tags:       gogit.AllTags,
	})
	if err != nil {
		if errors.Is(err, gogit.NoErrAlreadyUpToDate) {
			return opOK("Already up to date.")
		}
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return opErr(errors.New("remote repository is empty"))
		}
		return opFail(err, authDesc)
	}

	res := opOK("Fetch completed successfully.")
	if authDesc != "" {
		res.Auth = authDesc
	}
	return res
}

// GetRemotes lists the configured remotes of the repository.
func GetRemotes(workDir string) ([]GitRemote, error) {
	bundle, err := openRepo(workDir)
	if err != nil {
		return nil, err
	}
	remotes, err := bundle.repo.Remotes()
	if err != nil {
		return nil, err
	}
	result := make([]GitRemote, 0, len(remotes))
	for _, r := range remotes {
		result = append(result, GitRemote{Name: r.Config().Name, URLs: r.Config().URLs})
	}
	return result, nil
}

// pickRemote returns the remote named preferred, or the first remote when it
// does not exist, or nil when there are no remotes at all.
func pickRemote(repo *gogit.Repository, preferred string) *gogit.Remote {
	if preferred != "" {
		if r, err := repo.Remote(preferred); err == nil {
			return r
		}
	}
	remotes, err := repo.Remotes()
	if err != nil || len(remotes) == 0 {
		return nil
	}
	return remotes[0]
}

// opFail builds a failure result, adding a helpful hint when no credentials
// could be resolved.
func opFail(err error, authDesc string) GitOpResult {
	res := opErr(err)
	if authDesc == "" || authDesc == "none" {
		res.Error += "\n\nNo credentials were found on this system.\n" +
			"  - SSH remotes: start the OpenSSH Authentication Agent and add your key (ssh-add), or use ~/.ssh keys.\n" +
			"  - HTTPS remotes: store credentials with `git credential-manager` / Windows Credential Manager, or create a ~/.git-credentials file."
	}
	return res
}
