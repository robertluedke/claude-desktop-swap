package account

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetJSONReturnsErrUnauthorizedOn401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := getJSON[accountResponse](server.URL, "stale-session-key")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
}

func TestGetJSONReturnsGenericErrorOnOtherStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := getJSON[accountResponse](server.URL, "any-key")
	if err == nil || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want a non-ErrUnauthorized error", err)
	}
}

func TestUsageResponseParsesWindows(t *testing.T) {
	var usage usageResponse
	body := `{"five_hour":{"utilization":100.0,"resets_at":"2026-11-05T02:00:00.326693+00:00"},"seven_day":null}`
	if err := json.Unmarshal([]byte(body), &usage); err != nil {
		t.Fatal(err)
	}
	five := usage.FiveHour.window()
	if five.Percent != 100 || five.ResetsAt.UTC().Hour() != 2 {
		t.Fatalf("five_hour = %+v", five)
	}
	if seven := usage.SevenDay.window(); !seven.ResetsAt.IsZero() {
		t.Fatalf("null seven_day = %+v", seven)
	}
}
