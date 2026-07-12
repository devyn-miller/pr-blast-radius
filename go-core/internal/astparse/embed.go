// package astparse

// import "embed"

// // queryFS embeds every .scm file at build time, so the released binary
// // needs zero external files at runtime — this is what makes
// // "curl the binary and run it" a true zero-setup story (see project
// // README §CGO/distribution note).
// //
// //go:embed queries/go/*.scm queries/python/*.scm queries/typescript/*.scm
// var queryFS embed.FS

// // loadQuery reads one embedded .scm file's contents by language + name,
// // e.g. loadQuery(LangGo, "calls") -> queries/go/calls.scm contents.
// // Returns an error rather than panicking so a missing/renamed query file
// // is a clear startup error, not a silent nil-query bug at parse time.
// func loadQuery(lang Language, name string) (string, error) {
// 	path := "queries/" + string(lang) + "/" + name + ".scm"
// 	data, err := queryFS.ReadFile(path)
// 	if err != nil {
// 		return "", &QueryLoadError{Language: lang, QueryName: name, Path: path, Cause: err}
// 	}
// 	return string(data), nil
// }


// go-core/internal/astparse/embed.go
package astparse

import "embed"

//go:embed queries/go/*.scm queries/python/*.scm
var queryFS embed.FS

func loadQuery(lang Language, name string) (string, error) {
	path := "queries/" + string(lang) + "/" + name + ".scm"
	data, err := queryFS.ReadFile(path)
	if err != nil {
		return "", &QueryLoadError{Language: lang, QueryName: name, Path: path, Cause: err}
	}
	return string(data), nil
}