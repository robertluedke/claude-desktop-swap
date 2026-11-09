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
	Long: `Syncs Claude Code sessions between the primary profile and each other
profile now and on every switch, so every linked account shows and continues
the same sessions. Cowork sessions are not affected. A backup is written to
~/.claude-swap/sessions-backup-<time>.`,
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
			fmt.Printf("Claude Code sessions of %s are now synced with %q.\n", strings.Join(args[1:], ", "), args[0])
			return nil
		})
	},
}

var cmdUnlinkSessions = &cobra.Command{
	Use:   "unlink-code-sessions [profile...]",
	Short: "Give linked profiles their own Claude Code sessions again",
	Long: `Stops syncing sessions for each profile (all linked profiles if none
given). Each profile gets back the sessions it had before it was linked; the
primary profile keeps its own plus every session created while linked.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := profile.NewStore()
		if err != nil {
			return err
		}
		return withAppStopped(platform.Current(), os.Stdout, func(appData string) error {
			done, err := store.UnlinkSessions(appData, args)
			for _, r := range done {
				printUnlinkResult(os.Stdout, r)
			}
			if len(done) == 0 && err == nil {
				fmt.Println("No linked profiles.")
			}
			return err
		})
	},
}

func printUnlinkResult(out io.Writer, r profile.UnlinkResult) {
	fmt.Fprintf(out, "Unlinked Claude Code sessions of %q from %q:\n", r.Profile, r.Primary)
	fmt.Fprintf(out, "  %s keeps %s from before the link and drops %s shared while linked.\n", r.Profile, sessionCount(r.Kept), sessionCount(r.Dropped))
	fmt.Fprintf(out, "  %s keeps %s, including every session created while linked, and hands back %s to %s.\n", r.Primary, sessionCount(r.PrimaryKept), sessionCount(r.Returned), r.Profile)
}

func sessionCount(n int) string {
	if n == 1 {
		return "1 session"
	}
	return fmt.Sprintf("%d sessions", n)
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
