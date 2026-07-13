package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/shyim/agm/internal/api"
	"github.com/shyim/agm/internal/credstore"
	"github.com/shyim/agm/internal/db"
	"github.com/shyim/agm/internal/paths"
	"github.com/shyim/agm/internal/target"
)

func init() {
	RootCmd.AddCommand(initCmd)
	RootCmd.AddCommand(loginCmd)
	RootCmd.AddCommand(importIDECmd)
	RootCmd.AddCommand(importCLICmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create data dir, key, and empty database",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := db.EnsureStore(); err != nil {
			return err
		}
		fmt.Printf("Standalone store ready:\n")
		fmt.Printf("  data dir: %s\n", paths.AgentDir())
		fmt.Printf("  database: %s\n", paths.CloudAccountsDBPath())
		fmt.Printf("  master key: %s\n", paths.MasterKeyPath())
		fmt.Println("\nNext: agm login  |  agm import-cli  |  agm import-ide")
		return nil
	},
}

var loginCmd = &cobra.Command{
	Use:     "login",
	Aliases: []string{"add"},
	Short:   "Add a Google account (browser OAuth)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Starting Google OAuth login…")
		acc, err := api.LoginAndSave(func(u string) {
			fmt.Println("Open this URL if the browser does not open:")
			fmt.Println(u)
			fmt.Println()
		})
		if err != nil {
			return err
		}
		fmt.Printf("Logged in as %s\n", acc.Email)
		if acc.Quota != nil {
			fmt.Println("Quota snapshot stored. Run: agm info " + acc.Email)
		}
		return nil
	},
}

var importIDECmd = &cobra.Command{
	Use:   "import-ide",
	Short: "Import account from Antigravity IDE (state.vscdb)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Importing active account from Antigravity IDE…")
		acc, err := api.ImportFromIDE()
		if err != nil {
			return err
		}
		fmt.Printf("Imported %s\n", acc.Email)
		_ = db.SetActiveForTarget(string(target.IDE), acc.ID)
		return nil
	},
}

var importCLICmd = &cobra.Command{
	Use:     "import-cli",
	Aliases: []string{"import-agy"},
	Short:   "Import account from Antigravity CLI credential store (agy)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Importing account from Antigravity CLI credential store…")
		tok, err := credstore.ReadToken()
		if err != nil {
			return fmt.Errorf("read credential store: %w", err)
		}
		acc, err := api.ImportToken(tok, "", "")
		if err != nil {
			return err
		}
		fmt.Printf("Imported %s (from agy keychain)\n", acc.Email)
		_ = db.SetActiveForTarget(string(target.Agy), acc.ID)
		_ = db.SetActive(acc.Email)
		return nil
	},
}
