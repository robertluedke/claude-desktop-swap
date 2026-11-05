package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/FranCalveyra/claude-desktop-swap/internal/profile"
)

// ErrUnauthorized indicates the server explicitly rejected the sessionKey
// (HTTP 401), distinct from an indeterminate failure (offline, timeout).
var ErrUnauthorized = errors.New("session rejected by server")

const (
	orgsEndpoint    = "https://claude.ai/api/organizations"
	usagePath       = "/usage"
	accountEndpoint = "https://claude.ai/api/account"
	userAgent       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var defaultClient = &http.Client{Timeout: 10 * time.Second}

// orgResponse covers the known shapes of the /api/organizations response.
type orgResponse struct {
	UUID         string          `json:"uuid"`
	Capabilities json.RawMessage `json:"capabilities"`
	RavenType    string          `json:"raven_type"`
	Settings     struct {
		Tier string `json:"tier"`
	} `json:"settings"`
	BillingInfo struct {
		Tier string `json:"tier"`
		Plan string `json:"plan"`
	} `json:"billing_info"`
}

type usageWindow struct {
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at"`
}

type usageResponse struct {
	FiveHour *usageWindow `json:"five_hour"`
	SevenDay *usageWindow `json:"seven_day"`
}

func (w *usageWindow) window() profile.UsageWindow {
	if w == nil {
		return profile.UsageWindow{}
	}
	return profile.UsageWindow{Percent: w.Utilization, ResetsAt: w.ResetsAt}
}

type accountResponse struct {
	Email string `json:"email_address"`
}

// fetchFromAPI calls the Claude.ai API using a decrypted sessionKey cookie and
// returns the best-effort email and plan for the account.
func fetchFromAPI(sessionKey string) (Info, error) {
	info := Info{}

	// The account holds several orgs (chat, API, ...); take the first that
	// resolves to a subscription plan.
	orgs, err := getJSON[[]orgResponse](orgsEndpoint, sessionKey)
	if errors.Is(err, ErrUnauthorized) {
		info.Rejected = true
	} else if err == nil && len(orgs) > 0 {
		usageOrg := orgs[0].UUID
		for _, org := range orgs {
			if plan := parsePlan(org); plan != "" {
				info.Plan, usageOrg = plan, org.UUID
				break
			}
		}
		if usage, err := getJSON[usageResponse](orgsEndpoint+"/"+usageOrg+usagePath, sessionKey); err == nil {
			info.FiveHour, info.SevenDay = usage.FiveHour.window(), usage.SevenDay.window()
		}
	}

	acc, err := getJSON[accountResponse](accountEndpoint, sessionKey)
	if errors.Is(err, ErrUnauthorized) {
		info.Rejected = true
	} else if err == nil {
		info.Email = acc.Email
	}

	return info, nil
}

// getJSON performs an authenticated GET request and unmarshals the JSON body into T.
func getJSON[T any](url, sessionKey string) (T, error) {
	var zero T
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return zero, err
	}
	req.Header.Set("Cookie", "sessionKey="+sessionKey)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://claude.ai/")

	resp, err := defaultClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return zero, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return zero, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}
	var result T
	return result, json.Unmarshal(body, &result)
}
