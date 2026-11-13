// Logic of the GitHub plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Repo is the part of a GitHub repository we keep.
type Repo struct {
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Description string `json:"description"`
	Language    string `json:"language"`
	Visibility  string `json:"visibility"`
	Stars       int    `json:"stargazers_count"`
	OpenIssues  int    `json:"open_issues_count"`
	Archived    bool   `json:"archived"`
	Fork        bool   `json:"fork"`
	PushedAt    string `json:"pushed_at"`
	License     *struct {
		SPDX string `json:"spdx_id"`
	} `json:"license"`
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

func validName(s string) error {
	if !nameRe.MatchString(s) {
		return fmt.Errorf("%q is not a GitHub name", s)
	}
	return nil
}

func parseRepos(body []byte) ([]Repo, error) {
	var list []Repo
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("unexpected answer from GitHub: %w", err)
	}
	return list, nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// graphOps stores repositories as github_repo entries; forks are skipped.
func graphOps(owner string, repos []Repo) pdk.GraphOps {
	var ops pdk.GraphOps
	for _, r := range repos {
		if r.Fork {
			continue
		}
		attrs := map[string]any{"url": r.HTMLURL, "description": r.Description, "language": r.Language, "visibility": r.Visibility,
			"stars": r.Stars, "open_issues": r.OpenIssues, "archived": r.Archived}
		if len(r.PushedAt) >= 10 {
			attrs["pushed_at"] = r.PushedAt[:10]
		}
		if r.License != nil && r.License.SPDX != "" && r.License.SPDX != "NOASSERTION" {
			attrs["license"] = r.License.SPDX
		}
		ops.UpsertNamed("github_repo", "gh-"+slugify(owner)+"-"+slugify(r.Name), owner+"/"+r.Name, attrs)
	}
	return ops
}

// Item is an open issue or pull request.
type Item struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"html_url"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PR *struct{} `json:"pull_request,omitempty"`
}

// OpenItem is a line of github.open.
type OpenItem struct {
	Kind   string   `json:"kind"` // issue | pull
	Number int      `json:"number"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Author string   `json:"author"`
	Labels []string `json:"labels,omitempty"`
}

func openItems(body []byte) ([]OpenItem, error) {
	var list []Item
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("unexpected answer from GitHub: %w", err)
	}
	out := []OpenItem{}
	for _, it := range list {
		kind := "issue"
		if it.PR != nil {
			kind = "pull"
		}
		o := OpenItem{Kind: kind, Number: it.Number, Title: it.Title, URL: it.URL, Author: it.User.Login}
		for _, l := range it.Labels {
			o.Labels = append(o.Labels, l.Name)
		}
		out = append(out, o)
	}
	return out, nil
}

func jsonUnmarshal(b []byte, v any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, v)
}
