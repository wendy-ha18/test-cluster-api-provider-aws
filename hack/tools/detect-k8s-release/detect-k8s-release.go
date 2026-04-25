// Package main provides utilities for detecting the latest stable Kubernetes
// releases from the kubernetes/kubernetes GitHub repository.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	githubTagsURL = "https://api.github.com/repos/kubernetes/kubernetes/tags"
	// topMinorCount is the number of minor versions to track, per the CAPA AMI
	// publication policy: https://cluster-api-aws.sigs.k8s.io/topics/images/built-amis#ami-publication-policy
	topMinorCount = 3
)

// stableTagRe matches only stable release tags like v1.32.3.
// It explicitly excludes pre-release suffixes (alpha, beta, rc).
var stableTagRe = regexp.MustCompile(`^v1\.(\d+)\.(\d+)$`)

// githubTag is one entry from the GitHub tags API response.
type githubTag struct {
	Name string `json:"name"`
}

// MinorVersion groups all patch releases under a single minor version.
type MinorVersion struct {
	Minor   string   `json:"minor"`
	Patches []string `json:"patches"`
}

// SupportedVersions is the top-level structure written to the JSON file.
type SupportedVersions struct {
	GeneratedAt string         `json:"generated_at"`
	Versions    []MinorVersion `json:"versions"`
}

// fetchAllTags retrieves every tag from the kubernetes/kubernetes repository
// via the GitHub REST API, handling pagination automatically.
// An optional personal-access token can be provided to raise the API rate limit.
func fetchAllTags(token string) ([]string, error) {
	var names []string

	client := &http.Client{Timeout: 30 * time.Second}

	for page := 1; ; page++ {
		url := fmt.Sprintf("%s?per_page=100&page=%d", githubTagsURL, page)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching tags page %d: %w", page, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response body page %d: %w", page, err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d on page %d: %s", resp.StatusCode, page, body)
		}

		var tags []githubTag
		if err := json.Unmarshal(body, &tags); err != nil {
			return nil, fmt.Errorf("decoding tags page %d: %w", page, err)
		}

		if len(tags) == 0 {
			break
		}

		for _, t := range tags {
			names = append(names, t.Name)
		}
	}

	return names, nil
}

// filterStableTags returns only tags that represent a stable patch release
// (i.e. match vMAJOR.MINOR.PATCH with no pre-release suffix).
func filterStableTags(tags []string) []string {
	var stable []string
	for _, tag := range tags {
		if stableTagRe.MatchString(tag) {
			stable = append(stable, tag)
		}
	}
	return stable
}

// groupByMinor groups patch version strings (e.g. "1.32.3") by their minor
// component (e.g. "1.32"). The input tags are expected to have a leading "v".
func groupByMinor(tags []string) map[string][]string {
	groups := make(map[string][]string)
	for _, tag := range tags {
		// Strip leading "v" so we work with plain semver strings.
		ver := strings.TrimPrefix(tag, "v")
		parts := strings.SplitN(ver, ".", 3)
		if len(parts) != 3 {
			continue
		}
		minor := parts[0] + "." + parts[1]
		groups[minor] = append(groups[minor], ver)
	}
	return groups
}

// topMinors returns the N highest minor versions from the map, sorted
// numerically descending (e.g. ["1.32", "1.31", "1.30"]).
func topMinors(groups map[string][]string, n int) []string {
	minors := make([]string, 0, len(groups))
	for m := range groups {
		minors = append(minors, m)
	}

	sort.Slice(minors, func(i, j int) bool {
		return minorGreater(minors[i], minors[j])
	})

	if n > len(minors) {
		n = len(minors)
	}
	return minors[:n]
}

// minorGreater reports whether minor version a is greater than b.
// Both are expected in "MAJOR.MINOR" form (e.g. "1.32").
func minorGreater(a, b string) bool {
	aParts := strings.SplitN(a, ".", 2)
	bParts := strings.SplitN(b, ".", 2)
	if len(aParts) != 2 || len(bParts) != 2 {
		return a > b
	}
	aMaj, _ := strconv.Atoi(aParts[0])
	bMaj, _ := strconv.Atoi(bParts[0])
	if aMaj != bMaj {
		return aMaj > bMaj
	}
	aMin, _ := strconv.Atoi(aParts[1])
	bMin, _ := strconv.Atoi(bParts[1])
	return aMin > bMin
}

// sortPatchesDesc sorts patch version strings numerically descending in place.
// Strings are expected in "MAJOR.MINOR.PATCH" form (e.g. "1.32.3").
func sortPatchesDesc(patches []string) {
	sort.Slice(patches, func(i, j int) bool {
		return patchGreater(patches[i], patches[j])
	})
}

// patchGreater reports whether patch version a is greater than b.
func patchGreater(a, b string) bool {
	aParts := strings.SplitN(a, ".", 3)
	bParts := strings.SplitN(b, ".", 3)
	if len(aParts) != 3 || len(bParts) != 3 {
		return a > b
	}
	for idx := range 3 {
		av, _ := strconv.Atoi(aParts[idx])
		bv, _ := strconv.Atoi(bParts[idx])
		if av != bv {
			return av > bv
		}
	}
	return false
}

// detectSupportedVersions fetches Kubernetes tags from GitHub and returns the
// structured list of the top N supported minor versions with their patches.
func detectSupportedVersions(token string) (*SupportedVersions, error) {
	allTags, err := fetchAllTags(token)
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	stable := filterStableTags(allTags)
	if len(stable) == 0 {
		return nil, fmt.Errorf("no stable tags found")
	}

	groups := groupByMinor(stable)
	selected := topMinors(groups, topMinorCount)

	versions := make([]MinorVersion, 0, len(selected))
	for _, m := range selected {
		patches := groups[m]
		sortPatchesDesc(patches)
		versions = append(versions, MinorVersion{
			Minor:   m,
			Patches: patches,
		})
	}

	return &SupportedVersions{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Versions:    versions,
	}, nil
}
