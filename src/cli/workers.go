package cli

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"hash/crc32"

	"github.com/spf13/cobra"
)

var lowerBase32 = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

func newWorkersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workers",
		Short: "Manage workers",
	}
	cmd.AddCommand(newRegistrationTokensCommand())
	return cmd
}

func newRegistrationTokensCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registration-tokens",
		Short: "Manage worker registration tokens",
	}
	cmd.AddCommand(newRegistrationTokensGenerateCommand())
	return cmd
}

func newRegistrationTokensGenerateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "generate",
		Short: "Generate a worker registration token",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistrationTokensGenerate(cmd)
		},
	}
}

func runRegistrationTokensGenerate(cmd *cobra.Command) error {
	token, err := generateRegistrationToken()
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), token)
	return nil
}

func generateRegistrationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	encoded := lowerBase32.EncodeToString(b)
	tokenWithoutChecksum := "kwreg_" + encoded

	crc := crc32.ChecksumIEEE([]byte(tokenWithoutChecksum))
	checksum := fmt.Sprintf("%06x", crc&0xFFFFFF)

	return tokenWithoutChecksum + "_" + checksum, nil
}
