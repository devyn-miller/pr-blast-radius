package gitdiff

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
    OldPath    string // populated only for renames
    ChangeType ChangeType
    Hunks      []Hunk
}

// RefRange encapsulates the three supported input modes from FR1.
type RefRange struct {
    Kind    RefRangeKind
    FromRef string // e.g. "main", "HEAD~1" — unused when Kind == WorkingTree
    ToRef   string // e.g. "feature-branch", "HEAD"
}

type RefRangeKind string

const (
    RefRangeCommitRange RefRangeKind = "CommitRange"  // main...feature
    RefRangeSingleCommit RefRangeKind = "SingleCommit" // HEAD~1
    RefRangeWorkingTree RefRangeKind = "WorkingTree"   // uncommitted changes, incl. unstaged
)

type Extractor interface {
    ExtractDiff(ctx context.Context, repoPath string, rr RefRange) ([]FileDiff, string, error)
    // returns (diffs, resolvedCommitSHA, error) — resolvedCommitSHA populated
    // even for WorkingTree mode (uses current HEAD), feeds Graph.metadata.commit_sha
}

// go-git backed implementation
type GoGitExtractor struct{}
func NewGoGitExtractor() *GoGitExtractor
func (e *GoGitExtractor) ExtractDiff(ctx context.Context, repoPath string, rr RefRange) ([]FileDiff, string, error)