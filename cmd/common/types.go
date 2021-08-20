package common

import (
	"gitosint/pkg/git"
	"time"
)

// TODO:
type Error struct {
	Message string `json:"message,omitempty"`
}

type User struct {
	Login         string   `json:"login,omitempty"`
	Name          string   `json:"name,omitempty"`
	Type          string   `json:"type,omitempty"`
	Organizations []string `json:"orgs,omitempty"`
	Emails        []string `json:"emails,omitempty"`
}

type Repository struct {
	Owner          string              `json:"owner,omitempty"`
	RepositoryType string              `json:"type,omitempty"`
	Name           string              `json:"name,omitempty"`
	Fork           *bool               `json:"fork,omitempty"`
	Location       string              `json:"location,omitempty"`
	Metadata       map[string][]string `json:"metadata,omitempty"`
	CommitMetadata git.CommitMetadata  `json:"-"`
	Contributors   []*User             `json:"contributors,omitempty"`
}

type GitRecon struct {
	Time       time.Time   `json:"time"`
	Repository *Repository `json:"repository,omitempty"`
	User       *User       `json:"user,omitempty"`
	Error      []*Error    `json:"error,omitempty"`
}
