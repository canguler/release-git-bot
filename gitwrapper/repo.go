// Package gitwrapper wraps around git command.
package gitwrapper

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/src-d/go-git.v4/plumbing"

	"github.com/alecthomas/template"
	log "github.com/sirupsen/logrus"
	billy "gopkg.in/src-d/go-billy.v4"
	"gopkg.in/src-d/go-billy.v4/memfs"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/storage/memory"
)

// Repo represends a git repo.
type Repo struct {
	r        *git.Repository
	worktree *git.Worktree

	fs billy.Filesystem
}

// GithubClone creates a new Repo by cloning from github.
func GithubClone(owner, repo string) (*Repo, error) {
	url := fmt.Sprintf("https://github.com/%v/%v", owner, repo)

	fs := memfs.New()
	r, err := git.Clone(memory.NewStorage(), fs, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		return nil, err
	}

	worktree, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %v", err)
	}

	return &Repo{
		r:        r,
		worktree: worktree,
		fs:       fs,
	}, nil
}

func (r *Repo) createAndCheckoutBranch(name string) (*object.Commit, error) {
	ref, err := r.r.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to call Head(): %v", err)
	}

	err = r.worktree.Checkout(&git.CheckoutOptions{
		Hash:   ref.Hash(),
		Branch: plumbing.ReferenceName("refs/heads/" + name),
		Create: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to checkout to new branch: %v", err)
	}

	// newRef := plumbing.NewHashReference("refs/heads/my-branch", headRef.Hash())

	headCommit, err := r.r.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to find commit for head: %v", err)
	}
	return headCommit, nil
}

func (r *Repo) updateVersionFile(newVersion string) error {
	versionFile, err := r.fs.OpenFile("version.go", os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file version.go: %v", err)
	}
	defer versionFile.Close()

	t := template.Must(template.New("version").Parse(versionTemplate))
	err = t.Execute(versionFile, map[string]string{"version": newVersion})
	if err != nil {
		return fmt.Errorf("failed to execute template to file: %v", err)
	}
	return nil
}

func (r *Repo) printDiffInHeadCommit() error {
	headRef, err := r.r.Head()
	if err != nil {
		return fmt.Errorf("failed to call Head(): %v", err)
	}
	headCommit, err := r.r.CommitObject(headRef.Hash())
	if err != nil {
		return fmt.Errorf("failed to get head commit: %v", err)
	}
	parentCommit, err := headCommit.Parent(0)
	if err != nil {
		return fmt.Errorf("failed to get parent of head: %v", err)
	}

	// patch, err := parentTree.Patch(headTree)
	patch, err := parentCommit.Patch(headCommit)
	if err != nil {
		return fmt.Errorf("failed to get patch: %v", err)
	}
	log.Info(patch)
	return nil
}

// Try prints head.
func (r *Repo) Try() {
	// git checkout -b release_version
	headCommit, err := r.createAndCheckoutBranch("release_version")
	if err != nil {
		log.Fatal(err)
	}
	log.Info(headCommit.String())

	const newVersion = "1.new.0"

	// make change to file
	if err := r.updateVersionFile(newVersion); err != nil {
		log.Fatal(err)
	}
	status, err := r.worktree.Status()
	if err != nil {
		log.Fatalf("failed to get status from worktree: %v", err)
	}
	log.Infof("current worktree status: \n%v", status)

	// git commit -m 'Change version to %v'
	commitMsg := fmt.Sprintf("Change version to %v", newVersion)
	_, err = r.worktree.Commit(commitMsg, &git.CommitOptions{
		All: true,
		Author: &object.Signature{
			Name:  "release bot",
			Email: "releasebot@grpc.io",
			When:  time.Now(),
		},
	})
	if err != nil {
		log.Fatalf("failed to commit: %v", err)
	}

	if err := r.printDiffInHeadCommit(); err != nil {
		log.Fatal(err)
	}

	// git push -u
}
