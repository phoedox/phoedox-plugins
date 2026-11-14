// Logic of the GitLab plugin, without build tags so it is testable natively.
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/phoedox/phoedox-plugins/internal/integ"
	"github.com/phoedox/phoedox-sdk/go/pdk"
)

// Project is a GitLab project (GET /groups/:id/projects).
type Project struct {
	PathWithNamespace string   `json:"path_with_namespace"`
	Name              string   `json:"name"`
	WebURL            string   `json:"web_url"`
	Description       string   `json:"description"`
	Visibility        string   `json:"visibility"`
	DefaultBranch     string   `json:"default_branch"`
	Topics            []string `json:"topics"`
	StarCount         int      `json:"star_count"`
	OpenIssuesCount   *int     `json:"open_issues_count"`
	Archived          bool     `json:"archived"`
	LastActivityAt    string   `json:"last_activity_at"`
	ForkedFrom        any      `json:"forked_from_project"`
}

func parseProjects(body []byte) ([]Project, error) {
	var list []Project
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("unexpected answer from GitLab: %w", err)
	}
	return list, nil
}

var pathRe = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.-]*(/[A-Za-z0-9_.][A-Za-z0-9_.-]*)*$`)

// validPath accepts group and project paths (no "..", no empty parts).
func validPath(p string) error {
	if !pathRe.MatchString(p) || strings.Contains(p, "..") || len(p) > 255 {
		return fmt.Errorf("%q is not a GitLab path", p)
	}
	return nil
}

// graphOps: one gitlab_project per project; forks are left out.
func graphOps(projects []Project) pdk.GraphOps {
	var ops pdk.GraphOps
	for _, p := range projects {
		if p.ForkedFrom != nil {
			continue
		}
		attrs := map[string]any{"url": p.WebURL, "visibility": p.Visibility, "archived": p.Archived, "stars": p.StarCount}
		if p.Description != "" {
			attrs["description"] = p.Description
		}
		if p.DefaultBranch != "" {
			attrs["default_branch"] = p.DefaultBranch
		}
		if len(p.Topics) > 0 {
			attrs["topics"] = strings.Join(p.Topics, ", ")
		}
		if p.OpenIssuesCount != nil {
			attrs["open_issues"] = *p.OpenIssuesCount
		}
		if len(p.LastActivityAt) >= 10 {
			attrs["last_activity"] = p.LastActivityAt[:10]
		}
		ops.UpsertNamed("gitlab_project", "gl-"+integ.Slugify(p.PathWithNamespace), p.PathWithNamespace, attrs)
	}
	return ops
}

// Item is an open issue or merge request.
type Item struct {
	Kind   string   `json:"kind"` // issue | merge_request
	IID    int      `json:"iid"`
	Title  string   `json:"title"`
	URL    string   `json:"url"`
	Author string   `json:"author,omitempty"`
	Labels []string `json:"labels,omitempty"`
	Draft  bool     `json:"draft,omitempty"`
}

func parseItems(kind string, body []byte) ([]Item, error) {
	var raw []struct {
		IID    int      `json:"iid"`
		Title  string   `json:"title"`
		WebURL string   `json:"web_url"`
		Labels []string `json:"labels"`
		Draft  bool     `json:"draft"`
		Author *struct {
			Username string `json:"username"`
		} `json:"author"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unexpected answer from GitLab: %w", err)
	}
	out := make([]Item, 0, len(raw))
	for _, r := range raw {
		it := Item{Kind: kind, IID: r.IID, Title: r.Title, URL: r.WebURL, Labels: r.Labels, Draft: r.Draft}
		if r.Author != nil {
			it.Author = r.Author.Username
		}
		out = append(out, it)
	}
	return out, nil
}
