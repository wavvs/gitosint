package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"

	"gitosint/cmd/common"
	"gitosint/cmd/git"
	"gitosint/cmd/github"

	"github.com/spf13/cobra"
)

var (
	outputFile string
	//logFile    string
	ErrCmd error = errors.New("errCmd")
)

func NewCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gitrecon",
		Short: "Gitosint is a tool for reconnaissance of the Git services and extracting valuable metadata from commits.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if err := common.SetOutput(outputFile); err != nil {
				log.Fatal(err)
			}

			// f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			// if err != nil {
			// 	log.Fatal(err)
			// }
			// defer f.Close()

			// log.SetOutput(f)

			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt)
			go func() {
				<-stop
				//todo: add cleanup
				os.Exit(0)
			}()
		},
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		SilenceErrors:     true,
		SilenceUsage:      true,
	}

	rootCmd.Flags().SortFlags = false
	rootCmd.PersistentFlags().StringVarP(&outputFile, "output", "o", "", "Output file")
	//rootCmd.PersistentFlags().StringVarP(&logFile, "log", "l", "", "Log file")

	rootCmd.AddCommand(github.NewCommand())
	rootCmd.AddCommand(git.NewCommand())

	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		cmd.Printf("Error: %s\n", err)
		return ErrCmd
	})

	return rootCmd
}

func Execute() {
	rootCmd := NewCommand()
	if err := rootCmd.Execute(); err != nil {
		if err != ErrCmd {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
