package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/FranCalveyra/claude-desktop-swap/internal/platform"
	"github.com/FranCalveyra/claude-desktop-swap/internal/profile"
	"github.com/spf13/cobra"
)

var cmdList = &cobra.Command{
	Use:   "list",
	Short: "List saved profiles",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := profile.NewStore()
		if err != nil {
			return err
		}

		profiles, err := store.List()
		if err != nil {
			return err
		}

		if len(profiles) == 0 {
			fmt.Println("No profiles saved. Run 'claude-desktop-swap save <name>' to create one.")
			return nil
		}

		now := time.Now()
		current := ""
		if appData, err := platform.Current().AppDataPath(); err == nil {
			current, _ = store.MatchLive(appData)
			if live := liveSessionExpiry(appData, now); current != "" && !live.IsZero() {
				for i := range profiles {
					if profiles[i].Name == current {
						profiles[i].SessionExpiresAt = live
					}
				}
			}
		}
		enrichLiveAccounts(store, profiles)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tACCOUNT\tPLAN\t5H RESET\t7D RESET\tHEALTH\tLOGIN EXPIRES\tCREATED\tLAST USED")
		for _, p := range profiles {
			marker := " "
			if p.Name == current {
				marker = "*"
			}
			lastUsed := "never"
			if !p.LastUsed.IsZero() {
				lastUsed = p.LastUsed.Format("2006-01-02 15:04")
			}
			email := p.Email
			if email == "" {
				email = "-"
			}
			fmt.Fprintf(w, "%s %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				marker,
				p.Name,
				email,
				planLabel(p),
				usageLabel(p.FiveHour, now, "15:04"),
				usageLabel(p.SevenDay, now, "Mon 15:04"),
				healthLabel(p.ObservedHealth),
				metaExpiryLabel(p, now),
				p.CreatedAt.Format("2006-01-02 15:04"),
				lastUsed,
			)
		}
		if err := w.Flush(); err != nil {
			return err
		}

		notices := renewalNotices(profiles, now)
		if len(notices) > 0 {
			fmt.Println()
			for _, notice := range notices {
				fmt.Println(notice)
			}
		}
		return nil
	},
}

func healthLabel(health profile.Health) string { return string(health) }

// usageLabel shows when a usage window resets and how much of it is used. A
// window with no reset time (nothing used yet) or one already past has
// nothing to show.
func usageLabel(u profile.UsageWindow, now time.Time, layout string) string {
	if u.ResetsAt.IsZero() || !u.ResetsAt.After(now) {
		return "-"
	}
	return fmt.Sprintf("%s (%.0f%%)", u.ResetsAt.Local().Format(layout), u.Percent)
}
