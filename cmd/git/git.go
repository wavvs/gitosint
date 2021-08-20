package git

import (
	"errors"
	"gitosint/cmd/common"
	pkggit "gitosint/pkg/git"
	"os"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var opts options

func NewCommand() *cobra.Command {
	analyseCmd := &cobra.Command{
		Use:   "git",
		Short: "Extract metadata from Git repositories",
		RunE:  analyseMain,
	}

	opts = options{}
	opts.Username = analyseCmd.Flags().StringP("username", "u", "", "Git authentication username")
	opts.Token = analyseCmd.Flags().StringP("token", "t", "", "Git authentication token")
	opts.SshKeyPath = analyseCmd.Flags().String("ssh", "", "Path to the Git authentication key")
	opts.PassPrompt = analyseCmd.Flags().BoolP("pass", "p", false, "Password prompt")
	opts.GitRepos = analyseCmd.Flags().StringSlice("repos", []string{}, "Comma-delimited locations of Git repositories")
	opts.FGitRepos = analyseCmd.Flags().String("frepos", "", "Newline-delimited locations of Git repositories")
	opts.Local = analyseCmd.Flags().Bool("local", false, "Specify whether repository being analyzed is local")
	opts.Threads = analyseCmd.Flags().Int("threads", 10, "Concurrent cloning")
	analyseCmd.Flags().SortFlags = false
	return analyseCmd
}

func (o options) validate() error {
	if (len(*o.GitRepos) != 0 && *o.FGitRepos != "") || (len(*o.GitRepos) == 0 && *o.FGitRepos == "") {
		return errors.New("specify either --repos or --frepos")
	}

	if len(*o.GitRepos) == 0 && *o.FGitRepos != "" {
		lines, err := common.ReadFile(*o.FGitRepos)
		if err != nil {
			return err
		}

		o.GitRepos = &lines
	}

	if *o.Username != "" && *o.PassPrompt {
		templates := &promptui.PromptTemplates{
			Prompt:  "{{ . }}",
			Valid:   "{{ . }}",
			Success: "{{ . }}",
			Invalid: "{{ . }}",
		}
		prompt := promptui.Prompt{
			Label:       "Enter password: ",
			Mask:        '*',
			Templates:   templates,
			HideEntered: true,
		}
		result, err := prompt.Run()
		if err != nil {
			return err
		}
		*o.Token = result
	}

	if *o.Username != "" && *o.SshKeyPath != "" {
		if _, err := os.Stat(*o.SshKeyPath); err != nil {
			return err
		}
		err := pkggit.SetSSHAuth(*o.Username, *o.SshKeyPath, *o.Token)
		if err != nil {
			return err
		}
	} else if *o.Username != "" && *o.Token != "" {
		pkggit.SetBasicAuth(*o.Username, *o.Token)
	}

	return nil
}

func analyseMain(cmd *cobra.Command, args []string) error {
	if err := opts.validate(); err != nil {
		return err
	}

	var openResult []pkggit.OpenResult
	if !*opts.Local {
		for result := range pkggit.CloneRepos(*opts.GitRepos, *opts.Threads) {
			openResult = append(openResult, result)
		}
	} else {
		openResult = pkggit.OpenRepos(*opts.GitRepos)
	}

	for _, result := range openResult {
		record := &common.GitRecon{
			Time:       time.Now(),
			Repository: &common.Repository{Location: result.Origin},
		}
		if result.Error != nil {
			record.SetError(result.Error)
			record.Write()
			continue
		}

		metadata, err := pkggit.CollectMetadata(result.Repo)
		if err != nil {
			record.SetError(err)
			record.Write()
			continue
		}

		record.Repository.Metadata = pkggit.ConvertCommitMetadata(metadata)
		record.Write()
	}

	return nil
}
