package github

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/go-github/v35/github"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type Client struct {
	client *github.Client
}

func NewClient(token, baseURL, uploadURL string) (*Client, error) {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	var c *github.Client
	var err error
	if len(baseURL) == 0 || len(uploadURL) == 0 {
		c = github.NewClient(tc)
	} else {
		c, err = github.NewEnterpriseClient(baseURL, uploadURL, tc)
		if err != nil {
			return nil, err
		}
	}
	return &Client{client: c}, nil
}

func (c Client) GetUserOrOrganization(name string) (*github.User, error) {
	ctx := context.Background()
	user, _, err := c.client.Users.Get(ctx, name)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (c Client) GetUserOrganizations(name string) ([]*github.Organization,
	*github.Response, error) {
	ctx := context.Background()
	options := &github.ListOptions{PerPage: 100}
	var orgs []*github.Organization

	for {
		org, resp, err := c.client.Organizations.List(ctx, name, options)
		if err != nil {
			return orgs, resp, err
		}

		orgs = append(orgs, org...)
		if resp.NextPage == 0 {
			break
		}

		options.Page = resp.NextPage
	}

	return orgs, nil, nil
}

func (c Client) GetRepository(owner, repo string) (*github.Repository, *github.Response, error) {
	ctx := context.Background()
	repository, resp, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, resp, err
	}

	return repository, nil, nil
}

func (c Client) ListOrganizationMembers(org string) ([]*github.User,
	*github.Response, error) {
	ctx := context.Background()
	opts := &github.ListMembersOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var users []*github.User
	for {
		members, resp, err := c.client.Organizations.ListMembers(ctx, org, opts)
		if err != nil {
			return users, resp, err
		}

		users = append(users, members...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
	return users, nil, nil
}

func (c Client) ListRepositories(user, userType string, includeForks bool) ([]*github.Repository,
	*github.Response, error) {
	ctx := context.Background()
	options := github.ListOptions{PerPage: 100}
	optUser := &github.RepositoryListOptions{
		Type:        "all",
		ListOptions: options,
	}
	optOrg := &github.RepositoryListByOrgOptions{
		ListOptions: options,
	}

	var repos []*github.Repository
	var resp *github.Response
	var err error

	var allRepos []*github.Repository

	for {
		if userType == "User" {
			repos, resp, err = c.client.Repositories.List(ctx, user, optUser)
		} else {
			repos, resp, err = c.client.Repositories.ListByOrg(ctx, user, optOrg)
		}

		if err != nil {
			return allRepos, resp, err
		}

		for _, repo := range repos {
			if !*repo.Fork || includeForks {
				allRepos = append(allRepos, repo)
			}
		}

		if resp.NextPage == 0 {
			break
		}

		if userType == "User" {
			optUser.Page = resp.NextPage
		} else {
			optOrg.Page = resp.NextPage
		}
	}
	return allRepos, nil, nil
}

func (c Client) ListPullRequests(repo *github.Repository, count int) ([]*github.PullRequest, *github.Response, error) {
	ctx := context.Background()
	opts := &github.PullRequestListOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var allPulls []*github.PullRequest
	for {
		pulls, resp, err := c.client.PullRequests.List(ctx, *repo.Owner.Login, *repo.Name, opts)
		if err != nil {
			return allPulls, resp, err
		}

		allPulls = append(allPulls, pulls...)

		if count != 0 && len(allPulls) >= count {
			break
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return allPulls, nil, nil
}

// CreateRepository creates private Github repository with random name.
func (c Client) CreateRepository() (*github.Repository, error) {
	ctx := context.Background()
	githubRepo := &github.Repository{
		Name:    github.String(uuid.NewString()),
		Private: github.Bool(true),
	}
	createdGithubRepo, _, err := c.client.Repositories.Create(ctx, "", githubRepo)
	if err != nil {
		return nil, err
	}
	// back-off as recommended, 10 retries
	var sec time.Duration = 1
	for i := 0; i < 10; i++ {
		createdGithubRepo, _, err = c.client.Repositories.GetByID(ctx, *createdGithubRepo.ID)
		if err == nil {
			break
		}
		time.Sleep(sec * time.Second)
		sec *= 2
	}

	return createdGithubRepo, nil
}

// DeleteRepository deletes user's Github repository.
func (c Client) DeleteRepository(repo *github.Repository) error {
	ctx := context.Background()
	_, err := c.client.Repositories.Delete(ctx, *repo.Owner.Login, *repo.Name)
	return err
}

// ListContributors returns repository contributors (max 500)
func (c Client) ListContributors(repo *github.Repository) ([]*github.Contributor, *github.Response, error) {
	ctx := context.Background()
	opts := &github.ListContributorsOptions{
		Anon:        "",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var contributorsFinal []*github.Contributor
	for {
		contributors, resp, err := c.client.Repositories.ListContributors(ctx, *repo.Owner.Login, *repo.Name, opts)
		if err != nil {
			return contributorsFinal, resp, err
		}

		contributorsFinal = append(contributorsFinal, contributors...)

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return contributorsFinal, nil, nil
}

// ListEmails returns emails of a repository contributor
func (c Client) ListEmails(repo *github.Repository, contributor *github.Contributor) ([]string,
	*github.Response, error) {
	ctx := context.Background()
	emailsMap := make(map[string]struct{})
	opts := &github.CommitsListOptions{Author: *contributor.Login, ListOptions: github.ListOptions{PerPage: 100}}

	for {
		commits, resp, err := c.client.Repositories.ListCommits(ctx, *repo.Owner.Login, *repo.Name, opts)
		if err != nil {
			return nil, resp, err
		}

		if len(commits) == 0 {
			break
		}

		if commits[0].Commit.Author.Email != nil {
			emailsMap[*commits[0].Commit.Author.Email] = struct{}{}
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	emails := make([]string, 0, len(emailsMap))
	for k := range emailsMap {
		emails = append(emails, k)
	}
	return emails, nil, nil
}

func (c Client) searchCommits(query string, opts github.SearchOptions) (
	*github.CommitsSearchResult, *github.Response, error) {
	ctx := context.Background()
	commits := github.CommitsSearchResult{}
	for {
		csr, resp, err := c.client.Search.Commits(ctx, query, &opts)
		if err != nil {
			if _, ok := err.(*github.RateLimitError); ok {
				limits, _, _ := c.client.RateLimits(ctx) // dont care about error
				if limits.GetCore().Remaining == 0 {
					// hitting global rate limit
					return &commits, resp, err
				}
				currentTime := time.Now().UTC().Unix()
				delta := int64(math.Abs(float64(resp.Rate.Reset.UTC().Unix() - currentTime)))
				// Search rate limit hit
				time.Sleep(time.Duration(delta+1) * time.Second)
				continue
			}
			return &commits, resp, err
		}

		commits.Commits = append(commits.Commits, csr.Commits...)
		commits.Total = csr.Total
		commits.IncompleteResults = csr.IncompleteResults

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
	return &commits, nil, nil
}

func (c Client) searchPullRequests(query string, opts github.SearchOptions) (
	*github.IssuesSearchResult, *github.Response, error) {
	ctx := context.Background()
	pulls := github.IssuesSearchResult{}
	query += " is:pr"
	for {
		isr, resp, err := c.client.Search.Issues(ctx, query, &opts)
		if err != nil {
			if _, ok := err.(*github.RateLimitError); ok {
				limits, _, _ := c.client.RateLimits(ctx)
				if limits.GetCore().Remaining == 0 {
					return &pulls, resp, err
				}
				currentTime := time.Now().UTC().Unix()
				delta := int64(math.Abs(float64(resp.Rate.Reset.UTC().Unix() - currentTime)))
				//Search rate limit hit
				time.Sleep(time.Duration(delta+1) * time.Second)
				continue
			}
			return &pulls, resp, err
		}

		pulls.Issues = append(pulls.Issues, isr.Issues...)
		pulls.Total = isr.Total
		pulls.IncompleteResults = isr.IncompleteResults

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}
	return &pulls, nil, nil
}

func (c Client) ListCommitsOnPullRequest(repo *github.Repository, number int) (
	[]*github.CommitResult, *github.Response, error) {
	ctx := context.Background()
	opts := &github.ListOptions{PerPage: 100}
	var commits []*github.CommitResult

	for {
		rc, resp, err := c.client.PullRequests.ListCommits(ctx, *repo.Owner.Login, *repo.Name, number, opts)
		if err != nil {
			return commits, resp, err
		}

		for _, commit := range rc {
			commits = append(commits, &github.CommitResult{
				SHA:         commit.SHA,
				Commit:      commit.Commit,
				Author:      commit.Author,
				Committer:   commit.Committer,
				Parents:     commit.Parents,
				HTMLURL:     commit.HTMLURL,
				URL:         commit.URL,
				CommentsURL: commit.CommentsURL,
				Repository:  repo,
			})
		}

		if resp.NextPage == 0 {
			break
		}

		opts.Page = resp.NextPage
	}

	return commits, nil, nil
}

func (c Client) ListCommits(repo *github.Repository, author string, opts github.ListOptions) (
	[]*github.RepositoryCommit, *github.Response, error) {
	ctx := context.Background()
	options := github.CommitsListOptions{
		Author:      author,
		ListOptions: opts,
	}

	var commits []*github.RepositoryCommit
	for {
		c, resp, err := c.client.Repositories.ListCommits(ctx, *repo.Owner.Login, *repo.Name, &options)
		if err != nil {
			return commits, resp, err
		}
		commits = append(commits, c...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return commits, nil, nil
}

// SearchCommits searches user commits and pull requests across Github
func (c Client) SearchCommits(loginOrEmail string, searchType string) ([]*github.CommitResult,
	*github.Response, error) {
	opts := github.SearchOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}

	if searchType != "author" && searchType != "committer" {
		return nil, nil, fmt.Errorf("invalid search type")
	}

	var commits []*github.CommitResult
	query := searchType + "%s:%s"

	orderOpts := []string{"desc", "asc"}
	sortOpts := []string{"", fmt.Sprintf("%s-date", searchType)}
	commitSet := make(map[string]struct{})

	if strings.Contains(loginOrEmail, "@") {
		query = fmt.Sprintf(query, "-email", loginOrEmail)
	} else {
		query = fmt.Sprintf(query, "", loginOrEmail)
	}

out:
	for _, cs := range sortOpts {
		for _, order := range orderOpts {
			opts.Sort = cs
			opts.Order = order
			csr, resp, err := c.searchCommits(query, opts)
			if err != nil {
				return commits, resp, err
			}
			for _, commit := range csr.Commits {
				if _, ok := commitSet[commit.GetSHA()]; !ok {
					commitSet[commit.GetSHA()] = struct{}{}
					commits = append(commits, commit)
				}
			}
			if csr.GetTotal() <= 1000 {
				break out
			}
		}
	}

	repos := make(map[string]*github.Repository)
	ctx := context.Background()
out2:
	for _, cs := range sortOpts {
		for _, order := range orderOpts {
			opts.Sort = cs
			opts.Order = order
			isr, resp, err := c.searchPullRequests(query, opts)
			if err != nil {
				return commits, resp, err
			}

			for _, pr := range isr.Issues {
				repo, ok := repos[*pr.RepositoryURL]
				if !ok {
					parsed := strings.Split(*pr.RepositoryURL, "/")
					repo, resp, err = c.client.Repositories.Get(ctx, parsed[len(parsed)-2], parsed[len(parsed)-1])
					if err != nil {
						return commits, resp, err
					}
				}
				repos[*pr.RepositoryURL] = repo
				cr, resp, err := c.ListCommitsOnPullRequest(repo, *pr.Number)
				if err != nil {
					return commits, resp, err
				}

				for _, commit := range cr {
					if _, ok := commitSet[commit.GetSHA()]; !ok {
						commitSet[commit.GetSHA()] = struct{}{}
						commits = append(commits, commit)
					}
				}
			}
			if isr.GetTotal() <= 1000 {
				break out2
			}
		}
	}
	return commits, nil, nil
}

func (c Client) RateLimits() (*github.RateLimits, *github.Response, error) {
	ctx := context.Background()
	return c.client.RateLimits(ctx)
}

func (c Client) IsGlobalRateLimitExceeded(err error) bool {
	if _, ok := err.(*github.RateLimitError); ok {
		limits, _, _ := c.client.RateLimits(context.Background())
		if limits.GetCore().Remaining == 0 {
			return true
		}
	}

	return false
}
