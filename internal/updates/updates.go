// Package updates asks GitHub for the latest TuxShell release.
//
// It only runs when the user presses "Check for updates" or has turned on the
// startup check. Nothing is downloaded or installed: the frontend links to the
// release page.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LatestURL is GitHub's "latest release" endpoint for this repo. It skips
// drafts and pre-releases.
const LatestURL = "https://api.github.com/repos/vgrigolaia/TuxShell/releases/latest"

// ReleasesURL is shown when the latest release has no page of its own.
const ReleasesURL = "https://github.com/vgrigolaia/TuxShell/releases"

// Result is what a check found.
type Result struct {
	// Latest is the newest released version, without the leading "v".
	Latest string `json:"latest"`
	// URL is the release page on github.com.
	URL string `json:"url"`
	// Newer is true when Latest is newer than the running version.
	Newer bool `json:"newer"`
}

// ErrUnavailable means GitHub couldn't be reached or gave no usable answer.
var ErrUnavailable = errors.New("couldn't reach GitHub to check for updates")

// Service is bound to the frontend.
type Service struct {
	url    string
	client *http.Client
}

// NewService returns a service that checks LatestURL.
func NewService() *Service {
	return &Service{url: LatestURL, client: &http.Client{Timeout: 10 * time.Second}}
}

// Check compares the running version with the latest release.
func (s *Service) Check(ctx context.Context, current string) (Result, error) {
	cur, ok := parse(current)
	if !ok {
		return Result{}, fmt.Errorf("unknown running version %q", current)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "TuxShell/"+current)
	resp, err := s.client.Do(req)
	if err != nil {
		return Result{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, ErrUnavailable
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Result{}, ErrUnavailable
	}
	latest, ok := parse(body.TagName)
	if !ok {
		return Result{}, ErrUnavailable
	}
	url := body.HTMLURL
	// Only ever hand the frontend a link into our own repo.
	if !strings.HasPrefix(url, ReleasesURL+"/") {
		url = ReleasesURL
	}
	return Result{
		Latest: strings.TrimPrefix(body.TagName, "v"),
		URL:    url,
		Newer:  newer(latest, cur),
	}, nil
}

// parse reads "1.2.3" or "v1.2.3" into its three numbers.
func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func newer(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
