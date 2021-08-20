package github

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"gitosint/cmd/common"
	"gitosint/pkg/git"
	"gitosint/pkg/github"
)

var opts options

func NewCommand() *cobra.Command {
	githubCmd := &cobra.Command{
		Use:     "github",
		Short:   "GitHub reconnaissance",
		Aliases: []string{"gh"},
		RunE:    githubMain,
	}

	githubCmd.Flags().SortFlags = false
	opts = options{}
	opts.Token = githubCmd.Flags().StringP("token", "t", "", "GitHub authentication token (required)")
	opts.Rate = githubCmd.Flags().Bool("rate", false, "Rate limits of the current token")
	opts.Users = githubCmd.Flags().StringSlice("users", []string{}, "Comma-delimited list of usernames")
	opts.Fusers = githubCmd.Flags().String("fusers", "", "File with newline-delimited list of usernames")
	opts.Emails = githubCmd.Flags().StringSlice("emails", []string{}, "Comma-delimited list of emails")
	opts.Femails = githubCmd.Flags().String("femails", "", "File with newline-delimited list of emails")
	opts.Repos = githubCmd.Flags().StringSlice("repos", []string{}, "Comma-delimited list of repositories (url)")
	opts.Frepos = githubCmd.Flags().String("frepos", "", "File with newline-delimited list of repositories")
	opts.Search = githubCmd.Flags().Bool("search", false, "Search pull requests and commits")
	opts.Forks = githubCmd.Flags().Bool("forks", false, "Include forked repositories")
	opts.Pulls = githubCmd.Flags().Bool("pulls", false, "Include pull requests")
	opts.Members = githubCmd.Flags().Bool("members", false, "Analyze organization members")
	opts.Contributors = githubCmd.Flags().Bool("contributors", false, "Include repository contributors")
	opts.MaxPullRequests = githubCmd.Flags().Int("max-pulls", 0, "Maximum number of pull requests")
	opts.BaseURL = githubCmd.Flags().String("baseurl", "https://api.github.com/", "GitHub Base API URL")
	opts.UploadURL = githubCmd.Flags().String("uploadurl", "https://uploads.github.com/", "GitHub Upload API URL")
	opts.Threads = githubCmd.Flags().Int("threads", 10, "Concurrent cloning")

	return githubCmd
}

func githubMain(cmd *cobra.Command, args []string) error {
	if err := opts.validate(); err != nil {
		return err
	}

	client, err := github.NewClient(*opts.Token, *opts.BaseURL, *opts.UploadURL)
	if err != nil {
		return fmt.Errorf("invalid client: (%s)", err)
	}

	currentUser, err := client.GetUserOrOrganization("")
	if err != nil {
		return fmt.Errorf("invalid token: (%s)", err)
	}
	gUser := currentUser.GetLogin()
	git.SetBasicAuth(gUser, *opts.Token)

	if *opts.Rate {
		limits, _, err := client.RateLimits()
		if err != nil {
			return err
		}
		common.PrintJSON(limits)
	} else if len(*opts.Emails) != 0 {
		output := make(chan *common.GitRecon)
		go bulkUserSearch(client, *opts.Emails, output)
		for record := range output {
			if err := record.Write(); err != nil {
				return err
			}
		}
		if *opts.Search {
			for _, email := range *opts.Emails {
				for _, record := range searchCommits(client, email) {
					record.Repository.Metadata = git.ConvertCommitMetadata(record.Repository.CommitMetadata)
					if err := record.Write(); err != nil {
						return err
					}
				}
			}
		}
	} else if len(*opts.Users) != 0 {
		output := make(chan *common.GitRecon)
		go bulkUserAnalysis(client, *opts.Users, output)

		for record := range output {
			if err := record.Write(); err != nil {
				return err
			}
		}
	} else if len(*opts.Repos) != 0 {
		output := make(chan *common.GitRecon)
		go bulkRepoAnalysis(client, *opts.Repos, output)

		for record := range output {
			if err := record.Write(); err != nil {
				return err
			}
		}
	} else {
		return errors.New("specify --users/--fusers, --repos/--frepos or --emails/--femails")
	}
	return nil
}
