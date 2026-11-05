package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/robertluedke/claude-desktop-swap/internal/platform"
	"github.com/robertluedke/claude-desktop-swap/internal/profile"
	"github.com/spf13/cobra"
)

var cmdLinkSessions = &cobra.Command{
	Use:   "link-code-sessions <primary> <profile>...",
	Short: "Share the primary profile's Claude Code sessions with other profiles",
	Long: `Merges each profile's Claude Code sessions into the primary profile's
session directory and replaces theirs with a link to it, so every linked
account shows and continues the same sessions. Cowork sessions are not
affected. A backup is written to ~/.claude-swap/sessions-backup-<time>.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := profile.NewStore()
		if err != nil {
			return err
		}
		return withAppStopped(platform.Current(), os.Stdout, func(appData string) error {
			if err := store.LinkSessions(appData, args[0], args[1:]); err != nil {
				return err
			}
			fmt.Printf("Claude Code sessions of %s now use %q's sessions.\n", strings.Join(args[1:], ", "), args[0])
			return nil
		})
	},
}

var cmdUnlinkSessions = &cobra.Command{
	Use:   "unlink-code-sessions [profile...]",
	Short: "Give linked profiles their own Claude Code sessions again",
	Long: `Removes the link for each profile (all linked profiles if none given).
Sessions a profile brought in when it was linked move back to it; sessions
created while linked stay with the primary profile.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := profile.NewStore()
		if err != nil {
			return err
		}
		return withAppStopped(platform.Current(), os.Stdout, func(appData string) error {
			done, err := store.UnlinkSessions(appData, args)
			if len(done) > 0 {
				fmt.Printf("Unlinked Claude Code sessions of %s.\n", strings.Join(done, ", "))
			} else if err == nil {
				fmt.Println("No linked profiles.")
			}
			return err
		})
	},
}

// withAppStopped runs fn with Claude Desktop stopped, restarting it afterwards
// if it was running, because the app holds session files open and rewrites them.
func withAppStopped(p platform.Platform, out io.Writer, fn func(appData string) error) error {
	appData, err := p.AppDataPath()
	if err != nil {
		return err
	}
	running, err := p.IsRunning()
	if err != nil {
		return err
	}
	if running {
		fmt.Fprintln(out, "Stopping Claude Desktop...")
		if err := p.KillApp(); err != nil {
			return fmt.Errorf("stop Claude: %w", err)
		}
	}
	fnErr := fn(appData)
	if running {
		fmt.Fprintln(out, "Starting Claude Desktop...")
		if err := p.LaunchApp(); err != nil && fnErr == nil {
			return fmt.Errorf("could not restart Claude; launch manually: %w", err)
		}
	}
	return fnErr
}
