package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/share"
)

var pushPublic bool

var pushCmd = &cobra.Command{
	Use:   "push <path>",
	Short: "Push a Thinkt to wethinkt.com",
	Long:  "Upload a Thinkt object to wethinkt.com for private storage or public sharing.",
	Args:  cobra.ExactArgs(1),
	RunE:  runPush,
}

func runPush(cmd *cobra.Command, args []string) error {
	thinktPath := args[0]

	// Load credentials — trigger login if not found
	credsPath := share.DefaultCredentialsPath()
	creds, err := share.LoadCredentials(credsPath)
	if err != nil {
		fmt.Println("Not logged in. Starting login flow...")
		if loginErr := runLogin(cmd, nil); loginErr != nil {
			return loginErr
		}
		creds, err = share.LoadCredentials(credsPath)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}
	}

	// Read the trace file
	data, err := os.ReadFile(thinktPath)
	if err != nil {
		return fmt.Errorf("read trace: %w", err)
	}

	visibility := "private"
	if pushPublic {
		visibility = "public"
	}

	title := filepath.Base(thinktPath)

	client := share.NewUploadClient(creds)
	fmt.Printf("Pushing to wethinkt.com (%s)...\n", visibility)

	resp, err := client.Upload(data, visibility, title)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n", resp.URL)
	if visibility == "private" {
		fmt.Println("(private — only you can view)")
	}
	return nil
}
