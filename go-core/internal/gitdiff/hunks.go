// go-core/internal/gitdiff/hunks.go
package gitdiff

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
)

// gitDiffFile is the minimal interface both go-git's diff.File values
// satisfy — declared locally so classifyFilePatch (in extractor.go)
// doesn't need to import go-git's diff package just to accept its type.
type gitDiffFile interface {
	Path() string
	Mode() filemode.FileMode
}

func extractHunksFromChunks(filePatch diff.FilePatch) []Hunk {
	var hunks []Hunk
	toLine := 1

	for _, chunk := range filePatch.Chunks() {
		lineCount := strings.Count(chunk.Content(), "\n")
		if !strings.HasSuffix(chunk.Content(), "\n") && chunk.Content() != "" {
			lineCount++
		}

		switch chunk.Type() {
		case diff.Equal:
			toLine += lineCount
		case diff.Add:
			hunks = append(hunks, Hunk{StartLine: toLine, EndLine: toLine + lineCount - 1})
			toLine += lineCount
		case diff.Delete:
			hunks = append(hunks, Hunk{StartLine: toLine, EndLine: toLine})
		}
	}

	return mergeAdjacentHunks(hunks)
}

func mergeAdjacentHunks(hunks []Hunk) []Hunk {
	if len(hunks) <= 1 {
		return hunks
	}
	merged := []Hunk{hunks[0]}
	for _, h := range hunks[1:] {
		last := &merged[len(merged)-1]
		if h.StartLine <= last.EndLine+1 {
			if h.EndLine > last.EndLine {
				last.EndLine = h.EndLine
			}
			continue
		}
		merged = append(merged, h)
	}
	return merged
}

func (e *GoGitExtractor) computeHunksForFile(repo *git.Repository, filePath string) ([]Hunk, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var oldContent string
	f, err := tree.File(filePath)
	if err == nil {
		oldContent, _ = f.Contents()
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	newBytes, err := readWorktreeFile(wt, filePath)
	if err != nil {
		return nil, err
	}

	return diffTextToHunks(oldContent, string(newBytes)), nil
}

func diffTextToHunks(oldText, newText string) []Hunk {
	oldLines := splitLines(oldText)
	newLines := splitLines(newText)

	prefixLen := 0
	for prefixLen < len(oldLines) && prefixLen < len(newLines) && oldLines[prefixLen] == newLines[prefixLen] {
		prefixLen++
	}

	oldSuffix, newSuffix := len(oldLines), len(newLines)
	for oldSuffix > prefixLen && newSuffix > prefixLen && oldLines[oldSuffix-1] == newLines[newSuffix-1] {
		oldSuffix--
		newSuffix--
	}

	if prefixLen >= newSuffix {
		return nil
	}

	return []Hunk{{StartLine: prefixLen + 1, EndLine: newSuffix}}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func readWorktreeFile(wt *git.Worktree, path string) ([]byte, error) {
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return nil, fmt.Errorf("gitdiff: reading worktree file %s: %w", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	return []byte(sb.String()), scanner.Err()
}