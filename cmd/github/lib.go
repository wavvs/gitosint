package github

import (
	"errors"
	"fmt"
	"gitosint/cmd/common"
	"gitosint/pkg/git"
	"gitosint/pkg/github"
	"os"
	"strings"
	"time"

	gh "github.com/google/go-github/v35/github"
)

func (o options) validate() error {
	if *o.Token == "" {
		return errors.New("specify -t/--token")
	}

	if len(*opts.Users) != 0 && *opts.Fusers != "" {
		return errors.New("use either --users or --fusers")
	}

	if len(*opts.Emails) != 0 && *opts.Femails != "" {
		return errors.New("use either --emails or --femails")
	}

	if len(*opts.Repos) != 0 && *opts.Frepos != "" {
		return errors.New("use either --repos or --frepos")
	}

	if len(*opts.Users) == 0 && *opts.Fusers != "" {
		lines, err := common.ReadFile(*opts.Fusers)
		if err != nil {
			return err
		}
		opts.Users = &lines
	} else if len(*opts.Emails) == 0 && *opts.Femails != "" {
		lines, err := common.ReadFile(*opts.Femails)
		if err != nil {
			return err
		}
		opts.Emails = &lines
	} else if len(*opts.Repos) == 0 && *opts.Frepos != "" {
		lines, err := common.ReadFile(*opts.Frepos)
		if err != nil {
			return err
		}
		opts.Repos = &lines
	}

	return nil
}

func bulkUserAnalysis(client *github.Client, users []string, out chan<- *common.GitRecon) {
	var err error = nil
	var gUser *gh.User
	gUser, err = client.GetUserOrOrganization("")
	if err != nil {
		record := &common.GitRecon{}
		record.SetError(fmt.Errorf("failed to get authenticated user: (%s)", err.Error()))
		out <- record
		return
	}

	authUserLogin := gUser.GetLogin()

	for _, user := range users {
		if !client.IsGlobalRateLimitExceeded(err) {
			gUser, err = client.GetUserOrOrganization(user)
		}
		if err != nil {
			record := &common.GitRecon{}
			record.SetError(fmt.Errorf("failed to get user '%s': (%s)", user, err.Error()))
			out <- record
			continue
		}

		if gUser.GetLogin() == authUserLogin {
			*gUser.Login = ""
		}

		analyseUser(client, gUser, out)
	}

	close(out)
}

func analyseUser(client *github.Client, user *gh.User, out chan<- *common.GitRecon) {
	var login string
	if *user.Login == "" {
		parsed := strings.Split(*user.HTMLURL, "/")
		login = parsed[len(parsed)-1]
	} else {
		login = *user.Login
	}

	analysedUser := &common.User{
		Login: login,
		Type:  *user.Type,
	}

	if user.Email != nil {
		analysedUser.Emails = []string{*user.Email}
	}

	if user.Name != nil {
		analysedUser.Name = *user.Name
	}

	record := &common.GitRecon{
		User: analysedUser,
	}
	if user.GetType() == "User" {
		orgs, _, err := client.GetUserOrganizations(user.GetLogin())
		if err != nil {
			record.SetError(fmt.Errorf("failed to get organizations for '%s': (%s)",
				user.GetLogin(), err.Error()))
			out <- record
			return
		}
		for _, org := range orgs {
			record.User.Organizations = append(record.User.Organizations, *org.Login)
		}
	} else {
		orgMembers, _, err := client.ListOrganizationMembers(user.GetLogin())
		if err != nil {
			record.SetError(fmt.Errorf("failed to list organization's members for '%s': (%s)",
				user.GetLogin(), err.Error()))
			out <- record
			return
		}

		memberRecordFunc := func(ghUser *gh.User) *common.GitRecon {
			record := &common.GitRecon{
				Time: time.Now(),
				User: &common.User{
					Login: *ghUser.Login,
					Type:  *ghUser.Type,
				},
			}
			if ghUser.Email != nil {
				record.User.Emails = []string{*ghUser.Email}
			}
			if ghUser.Name != nil {
				record.User.Name = *ghUser.Name
			}
			return record
		}

		for _, member := range orgMembers {
			var ghUser *gh.User
			if !client.IsGlobalRateLimitExceeded(err) {
				ghUser, err = client.GetUserOrOrganization(member.GetLogin())
			}

			if err != nil {
				record := memberRecordFunc(member)
				record.SetError(err)
				out <- record
				continue
			}

			if *opts.Members {
				analyseUser(client, ghUser, out)
			} else {
				record := memberRecordFunc(ghUser)
				out <- record
			}
		}
	}

	repos, _, err := client.ListRepositories(user.GetLogin(), user.GetType(), *opts.Forks)
	if err != nil {
		record.SetError(fmt.Errorf("failed to list repositories for '%s': (%s)",
			user.GetLogin(), err.Error()))
		out <- record
		return
	}

	seenRepos := make(map[string]struct{})
	for _, repo := range repos {
		seenRepos[*repo.HTMLURL] = struct{}{}
	}

	if user.GetType() == "User" && *opts.Search {
		for _, record := range searchCommits(client, login) {
			if _, ok := seenRepos[record.Repository.Location]; !ok {
				record.User = analysedUser
				if record.Repository.CommitMetadata != nil {
					record.Repository.Metadata = git.ConvertCommitMetadata(record.Repository.CommitMetadata)
				}
				out <- record
			}
		}
	}

	outCh := make(chan *common.GitRecon)
	go analyseRepos(client, repos, outCh)
	for record := range outCh {
		record.User = analysedUser
		out <- record
	}
}

func bulkRepoAnalysis(client *github.Client, repos []string, out chan<- *common.GitRecon) {
	gUser, err := client.GetUserOrOrganization("")
	if err != nil {
		record := &common.GitRecon{}
		record.SetError(fmt.Errorf("failed to get authenticated user: (%s)", err.Error()))
		out <- record
		return
	}

	var authUserRepos map[string]*gh.Repository
	var ghRepos []*gh.Repository
	for _, repo := range repos {
		parsed := strings.Split(repo, "/")
		owner := parsed[len(parsed)-2]
		repo := parsed[len(parsed)-1]
		if owner == *gUser.Login {
			if authUserRepos == nil {
				authUserRepos = make(map[string]*gh.Repository)
				authRepos, _, err := client.ListRepositories("", *gUser.Type, true)
				if err != nil {
					record := &common.GitRecon{}
					record.SetError(fmt.Errorf("failed to get authenticated user repos: (%s)", err.Error()))
					out <- record
					return
				}
				for _, authRepo := range authRepos {
					authUserRepos[*authRepo.Name] = authRepo
				}
			}
			r, ok := authUserRepos[repo]
			if ok {
				ghRepos = append(ghRepos, r)
			}
		} else {
			r, _, err := client.GetRepository(owner, repo)
			if err != nil {
				record := &common.GitRecon{}
				record.SetError(fmt.Errorf("failed to get repo '%s/%s': (%s)", owner, repo, err.Error()))
				out <- record
				continue
			}
			ghRepos = append(ghRepos, r)
		}
	}

	analyseRepos(client, ghRepos, out)
}

func analyseRepos(client *github.Client, repos []*gh.Repository, out chan<- *common.GitRecon) {
	// TODO: rewrite
	defer close(out)
	var err error = nil
	var urls []string
	urlToRepo := make(map[string]*common.GitRecon)
	for _, repo := range repos {
		urls = append(urls, *repo.HTMLURL)
		record := &common.GitRecon{
			Repository: &common.Repository{
				Owner:          *repo.Owner.Login,
				Location:       *repo.HTMLURL,
				Name:           *repo.Name,
				Fork:           repo.Fork,
				CommitMetadata: make(git.CommitMetadata),
			},
		}

		if *opts.Pulls {
			var pulls []*gh.PullRequest
			var allCommits []*gh.CommitResult
			if !client.IsGlobalRateLimitExceeded(err) {
				pulls, _, err = client.ListPullRequests(repo, *opts.MaxPullRequests)
			}
			if err != nil {
				record.SetError(fmt.Errorf("failed to list pull requests for '%s': (%s)",
					*repo.HTMLURL, err.Error()))
				continue
			}
			for _, pull := range pulls {
				if pull.MergedAt != nil {
					continue
				}
				if !client.IsGlobalRateLimitExceeded(err) {
					commits, _, err := client.ListCommitsOnPullRequest(repo, *pull.Number)
					if err == nil {
						allCommits = append(allCommits, commits...)
					}
				}
				if err != nil {
					record.SetError(fmt.Errorf("failed to list commits on pull request '%s': (%s)",
						*pull.HTMLURL, err.Error()))
					continue
				}
			}

			processedMetadata, _ := processCommits(allCommits, "")
			if processedMetadata[*repo.ID] != nil {
				record.Repository.CommitMetadata = processedMetadata[*repo.ID]
			}
		}

		urlToRepo[*repo.HTMLURL] = record
	}

	recordCh := make(chan *common.GitRecon)
	go func() {
		for result := range git.CloneRepos(urls, *opts.Threads) {
			record := urlToRepo[result.Origin]
			record.Time = time.Now()
			if result.Error != nil {
				record.SetError(fmt.Errorf("failed to clone '%s': (%s)",
					result.Origin, result.Error.Error()))
				recordCh <- record
				continue
			}

			metadata, err := git.CollectMetadata(result.Repo)
			if err != nil {
				record.SetError(fmt.Errorf("failed to extract metadata for '%s': (%s)",
					result.Origin, result.Error.Error()))
				recordCh <- record
				continue
			}
			os.RemoveAll(result.Dir)
			for k, v := range metadata {
				record.Repository.CommitMetadata[k] = v
			}
			recordCh <- record
		}
		close(recordCh)
	}()

	for record := range recordCh {
		record.Repository.Metadata = git.ConvertCommitMetadata(record.Repository.CommitMetadata)
		if *opts.Contributors {
			var emails []string
			for email := range record.Repository.Metadata {
				emails = append(emails, email)
			}
			outCh := make(chan *common.GitRecon)
			go bulkUserSearch(client, emails, outCh)
			for outRecord := range outCh {
				if outRecord.User != nil {
					record.Repository.Contributors = append(record.Repository.Contributors, outRecord.User)
				}
				if outRecord.Error != nil {
					record.Error = append(record.Error, outRecord.Error...)
				}
			}
		}
		out <- record
	}
}

func bulkUserSearch(client *github.Client, emails []string, out chan<- *common.GitRecon) {
	defer close(out)
	for i := 0; i < len(emails); i += 500 {
		end := i + 500
		if end > len(emails) {
			end = len(emails)
		}

		output := make(chan *common.GitRecon)
		go searchUsers(client, emails[i:end], output)

		for record := range output {
			out <- record
		}
	}
}

func searchUsers(client *github.Client, emails []string, out chan<- *common.GitRecon) {
	defer close(out)
	var err error = nil

	record := &common.GitRecon{}
	repo, err := client.CreateRepository()
	if err != nil {
		record.SetError(fmt.Errorf("failed to create remote repo: (%s)", err.Error()))
		out <- record
		return
	}
	defer client.DeleteRepository(repo)

	err = git.CreateRemoteRepo(emails, *repo.CloneURL)
	if err != nil {
		record.SetError(fmt.Errorf("failed to push emails to the remote repo: (%s)", err.Error()))
		out <- record
		return
	}
	// ensure repo is created
	time.Sleep(2 * time.Second)

	contributors, _, err := client.ListContributors(repo)
	if err != nil {
		record.SetError(fmt.Errorf("failed to list contributors: (%s)", err.Error()))
		out <- record
		return
	}

	var contribEmails []string
	for _, contributor := range contributors {
		record = &common.GitRecon{Time: time.Now()}
		user := &common.User{Login: contributor.GetLogin()}
		record.User = user
		if !client.IsGlobalRateLimitExceeded(err) {
			contribEmails, _, err = client.ListEmails(repo, contributor)
		}
		if err != nil {
			record.SetError(fmt.Errorf("failed to list emails for '%s': (%s)",
				contributor.GetLogin(), err.Error()))
			out <- record
			continue
		}

		record.User.Emails = contribEmails
		out <- record
	}
}

func searchCommits(client *github.Client, loginOrEmail string) []*common.GitRecon {
	var records []*common.GitRecon
	allRepos := make(map[int64]*gh.Repository)
	allMetadata := make(map[int64]git.CommitMetadata)

	for _, i := range []string{"author", "committer"} {
		commits, _, err := client.SearchCommits(loginOrEmail, i)
		if err != nil {
			record := &common.GitRecon{}
			record.SetError(fmt.Errorf("failed to search %s commits: (%s)", i, err.Error()))
			records = append(records, record)
			return records
		}
		metadata, repos := processCommits(commits, i)
		for id, gitmeta := range metadata {
			if _, ok := allMetadata[id]; ok {
				for k, v := range gitmeta {
					allMetadata[id][k] = v
				}
			} else {
				allMetadata[id] = gitmeta
			}
		}

		for _, repo := range repos {
			if _, ok := allRepos[*repo.ID]; !ok {
				allRepos[*repo.ID] = repo
			}
		}
	}

	for _, repo := range allRepos {
		record := &common.GitRecon{Time: time.Now()}
		record.Repository = &common.Repository{
			Owner:          *repo.Owner.Login,
			Name:           *repo.Name,
			Fork:           repo.Fork,
			Location:       *repo.HTMLURL,
			CommitMetadata: allMetadata[*repo.ID],
		}
		records = append(records, record)
	}

	return records
}

func processCommits(commits []*gh.CommitResult, usertype string) (map[int64]git.CommitMetadata, []*gh.Repository) {
	metadata := make(map[int64]git.CommitMetadata)
	var repos []*gh.Repository
	for _, commit := range commits {
		id := *commit.Repository.ID
		if _, ok := metadata[id]; !ok {
			metadata[id] = make(git.CommitMetadata)
			repos = append(repos, commit.Repository)
		}

		metadata[id] = make(git.CommitMetadata)
		if usertype == "author" {
			metadata[id][git.Metadata{
				Email: *commit.Commit.Author.Email,
				Name:  *commit.Commit.Author.Name}] = struct{}{}
		} else if usertype == "committer" {
			metadata[id][git.Metadata{
				Email: *commit.Commit.Committer.Email,
				Name:  *commit.Commit.Committer.Name}] = struct{}{}
		} else {
			metadata[id][git.Metadata{
				Email: *commit.Commit.Author.Email,
				Name:  *commit.Commit.Author.Name}] = struct{}{}
			metadata[id][git.Metadata{
				Email: *commit.Commit.Committer.Email,
				Name:  *commit.Commit.Committer.Name}] = struct{}{}
		}
	}

	return metadata, repos
}
