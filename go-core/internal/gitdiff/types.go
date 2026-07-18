package gitdiff

import "context"

type ChangeType string

const (
	ChangeAdded    ChangeType = "Added"
	ChangeModified ChangeType = "Modified"
	ChangeDeleted  ChangeType = "Deleted"
	ChangeRenamed  ChangeType = "Renamed"
)

type Hunk struct {
	StartLine int
	EndLine   int
}

type FileDiff struct {
	FilePath   string
	OldPath    string
	ChangeType ChangeType
	Hunks      []Hunk
}

type RefRangeKind string

const (
	RefRangeCommitRange  RefRangeKind = "CommitRange"
	RefRangeSingleCommit RefRangeKind = "SingleCommit"
	RefRangeWorkingTree  RefRangeKind = "WorkingTree"
)

type RefRange struct {
	Kind    RefRangeKind
	FromRef string
	ToRef   string
}

type Extractor interface {
	ExtractDiff(ctx context.Context, repoPath string, rr RefRange) ([]FileDiff, string, error)
}
