// go-core/internal/gitdiff/extractor.go
package gitdiff

import (
	"context"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type GoGitExtractor struct{}

func NewGoGitExtractor() *GoGitExtractor { return &GoGitExtractor{} }

func (e *GoGitExtractor) ExtractDiff(ctx context.Context, repoPath string, rr RefRange) ([]FileDiff, string, error) {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, "", fmt.Errorf("gitdiff: opening repo at %s: %w", repoPath, err)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, "", fmt.Errorf("gitdiff: resolving HEAD: %w", err)
	}
	headSHA := head.Hash().String()

	switch rr.Kind {
	case RefRangeWorkingTree:
		diffs, err := e.diffWorkingTree(repo)
		return diffs, headSHA, err
	case RefRangeSingleCommit, RefRangeCommitRange:
		diffs, err := e.diffCommits(repo, rr)
		return diffs, headSHA, err
	default:
		return nil, "", fmt.Errorf("gitdiff: unknown RefRangeKind %q", rr.Kind)
	}
}

func (e *GoGitExtractor) diffWorkingTree(repo *git.Repository) ([]FileDiff, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("gitdiff: getting worktree: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("gitdiff: getting status: %w", err)
	}

	var diffs []FileDiff
	for path, fileStatus := range status {
		if fileStatus.Worktree == git.Unmodified && fileStatus.Staging == git.Unmodified {
			continue
		}
		diffs = append(diffs, FileDiff{
			FilePath:   path,
			ChangeType: mapGitStatusCode(fileStatus.Worktree),
		})
	}

	for i := range diffs {
		hunks, err := e.computeHunksForFile(repo, diffs[i].FilePath)
		if err != nil {
			return nil, err
		}
		diffs[i].Hunks = hunks
	}

	return diffs, nil
}

func (e *GoGitExtractor) diffCommits(repo *git.Repository, rr RefRange) ([]FileDiff, error) {
	toCommit, err := resolveCommit(repo, rr.ToRef)
	if err != nil {
		return nil, err
	}

	var fromCommit *object.Commit
	if rr.Kind == RefRangeSingleCommit {
		parents := toCommit.ParentHashes
		if len(parents) == 0 {
			return nil, fmt.Errorf("gitdiff: %s has no parent commit to diff against", rr.ToRef)
		}
		fromCommit, err = repo.CommitObject(parents[0])
	} else {
		fromCommit, err = resolveCommit(repo, rr.FromRef)
	}
	if err != nil {
		return nil, err
	}

	fromTree, err := fromCommit.Tree()
	if err != nil {
		return nil, err
	}
	toTree, err := toCommit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return nil, fmt.Errorf("gitdiff: computing tree diff: %w", err)
	}

	var diffs []FileDiff
	for _, change := range changes {
		patch, err := change.Patch()
		if err != nil {
			return nil, err
		}
		for _, filePatch := range patch.FilePatches() {
			from, to := filePatch.Files()
			path, changeType := classifyFilePatch(from, to)
			diffs = append(diffs, FileDiff{
				FilePath:   path,
				ChangeType: changeType,
				Hunks:      extractHunksFromChunks(filePatch),
			})
		}
	}
	return diffs, nil
}

func resolveCommit(repo *git.Repository, ref string) (*object.Commit, error) {
	hash, err := repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		return nil, fmt.Errorf("gitdiff: resolving ref %q: %w", ref, err)
	}
	return repo.CommitObject(*hash)
}

func mapGitStatusCode(code git.StatusCode) ChangeType {
	switch code {
	case git.Added:
		return ChangeAdded
	case git.Deleted:
		return ChangeDeleted
	case git.Renamed:
		return ChangeRenamed
	default:
		return ChangeModified
	}
}

func classifyFilePatch(from, to gitDiffFile) (string, ChangeType) {
	if from == nil {
		return to.Path(), ChangeAdded
	}
	if to == nil {
		return from.Path(), ChangeDeleted
	}
	if from.Path() != to.Path() {
		return to.Path(), ChangeRenamed
	}
	return to.Path(), ChangeModified
}