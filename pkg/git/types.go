package git

import "github.com/go-git/go-git/v5"

const (
	remoteName    = "origin"
	tempDirPrefix = "gitrecon*"
)

type CommitMetadata map[Metadata]struct{}

type Metadata struct {
	Email string
	Name  string
}

type OpenResult struct {
	Origin string
	Repo   *git.Repository
	Dir    string
	Error  error
}

type AnalyseResult struct {
	Repo     *git.Repository
	Metadata CommitMetadata
	Error    error
}
