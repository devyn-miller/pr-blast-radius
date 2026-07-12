package astparse

import "fmt"

// UnsupportedFileError is returned (not panicked) when Registry.ParseFile
// is asked to parse a file extension with no registered Parser. Treated
// as a normal, expected condition by callers walking a full repo tree.
type UnsupportedFileError struct {
	FilePath string
}

func (e *UnsupportedFileError) Error() string {
	return fmt.Sprintf("astparse: no parser registered for file %q", e.FilePath)
}

// QueryLoadError indicates an embedded .scm file was missing or unreadable
// at startup — this should only ever happen due to a build misconfiguration
// (a query file renamed/deleted without updating embed.go), never at
// runtime from user input, so it's treated as a fatal startup error upstream.
type QueryLoadError struct {
	Language  Language
	QueryName string
	Path      string
	Cause     error
}

func (e *QueryLoadError) Error() string {
	return fmt.Sprintf("astparse: failed to load query %q for language %q at %q: %v",
		e.QueryName, e.Language, e.Path, e.Cause)
}

func (e *QueryLoadError) Unwrap() error { return e.Cause }