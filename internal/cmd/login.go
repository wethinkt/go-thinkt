package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wethinkt/go-thinkt/internal/share"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to wethinkt.com",
	Long:  "Authenticate with wethinkt.com using GitHub to enable pushing traces.",
	RunE:  runLogin,
}

func runLogin(cmd *cobra.Command, args []string) error {
	endpoint := share.DefaultEndpoint
	client := share.NewDeviceFlowClient(endpoint)

	fmt.Println("Requesting login code...")
	codeResp, err := client.RequestCode()
	if err != nil {
		return fmt.Errorf("failed to start login: %w", err)
	}

	fmt.Printf("\nGo to: %s\n", codeResp.VerificationURI)
	fmt.Printf("Enter code: %s\n\n", codeResp.UserCode)
	fmt.Println("Waiting for authorization...")

	tokenResp, err := client.PollForToken(codeResp.DeviceCode, codeResp.Interval)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	creds := &share.Credentials{
		Token:    tokenResp.Token,
		Username: tokenResp.User.Username,
		Endpoint: endpoint,
	}

	path := share.DefaultCredentialsPath()
	if err := share.SaveCredentials(path, creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Printf("Logged in as %s\n", tokenResp.User.Username)
	fmt.Printf("Credentials saved to %s\n", path)
	return nil
}
