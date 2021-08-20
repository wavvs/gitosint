package git

type options struct {
	GitRepos   *[]string
	FGitRepos  *string
	Local      *bool
	Username   *string
	Token      *string
	SshKeyPath *string
	PassPrompt *bool
	Threads    *int
}
