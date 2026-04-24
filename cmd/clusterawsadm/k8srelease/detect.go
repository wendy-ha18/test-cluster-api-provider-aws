/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package k8srelease provides utilities for detecting stable Kubernetes
// releases from the kubernetes/kubernetes GitHub repository.
package k8srelease

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	githubTagsURL = "https://api.github.com/repos/kubernetes/kubernetes/tags"
	// TopMinorCount is the number of minor versions tracked per the CAPA AMI
	// publication policy: https://cluster-api-aws.sigs.k8s.io/topics/images/built-amis#ami-publication-policy
	TopMinorCount = 3
)

// stableTagRe matches only stable release tags like v1.32.3.
// Pre-release suffixes (alpha, beta, rc) are intentionally not matched.
var stableTagRe = regexp.MustCompile(`^v1\.(\d+)\.(\d+)$`)

// githubTag is one entry from the GitHub tags API response.
type githubTag struct {
	Name string `json:"name"`
}

// MinorVersion groups all patch releases under a single Kubernetes minor version.
type MinorVersion struct {
	Minor   string   `json:"minor"`
	Patches []string `json:"patches"`
}

// SupportedVersions is the structured result of a CAPA-policy version query.
type SupportedVersions struct {
	GeneratedAt string         `json:"generated_at"`
	Versions    []MinorVersion `json:"versions"`
}

// ToTable converts a SupportedVersions into a metav1.Table for CLI rendering.
// Each row shows one minor version and its comma-separated patch list.
func (s *SupportedVersions) ToTable() *metav1.Table {
	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: metav1.SchemeGroupVersion.String(),
			Kind:       "Table",
		},
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Minor Version", Type: "string"},
			{Name: "Patch Versions", Type: "string"},
		},
	}
	for _, v := range s.Versions {
		table.Rows = append(table.Rows, metav1.TableRow{
			Cells: []interface{}{v.Minor, strings.Join(v.Patches, ", ")},
		})
	}
	return table
}

// ToTable converts a single MinorVersion into a metav1.Table for CLI rendering.
// Each row is one patch release.
func (m *MinorVersion) ToTable() *metav1.Table {
	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: metav1.SchemeGroupVersion.String(),
			Kind:       "Table",
		},
		ColumnDefinitions: []metav1.TableColumnDefinition{
			{Name: "Minor Version", Type: "string"},
			{Name: "Patch Version", Type: "string"},
		},
	}
	for _, p := range m.Patches {
		table.Rows = append(table.Rows, metav1.TableRow{
			Cells: []interface{}{m.Minor, p},
		})
	}
	return table
}

// DetectSupportedVersions fetches Kubernetes tags from GitHub and returns the
// top TopMinorCount minor versions with all their stable patch releases,
// following the CAPA AMI publication policy.
func DetectSupportedVersions(token string) (*SupportedVersions, error) {
	allTags, err := fetchAllTags(token)
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	stable := filterStableTags(allTags)
	if len(stable) == 0 {
		return nil, fmt.Errorf("no stable tags found")
	}

	groups := groupByMinor(stable)
	selected := topMinors(groups, TopMinorCount)

	versions := make([]MinorVersion, 0, len(selected))
	for _, m := range selected {
		patches := groups[m]
		sortPatchesDesc(patches)
		versions = append(versions, MinorVersion{Minor: m, Patches: patches})
	}

	return &SupportedVersions{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Versions:    versions,
	}, nil
}

// DetectVersionsForMinor fetches all stable patch releases for a single minor
// version (e.g. "1.32") from GitHub.
func DetectVersionsForMinor(minor, token string) (*MinorVersion, error) {
	allTags, err := fetchAllTags(token)
	if err != nil {
		return nil, fmt.Errorf("fetching tags: %w", err)
	}

	stable := filterStableTags(allTags)
	groups := groupByMinor(stable)

	patches, ok := groups[minor]
	if !ok || len(patches) == 0 {
		return nil, fmt.Errorf("no stable releases found for Kubernetes %s", minor)
	}

	sortPatchesDesc(patches)
	return &MinorVersion{Minor: minor, Patches: patches}, nil
}

// fetchAllTags retrieves every tag from the kubernetes/kubernetes repository
// via the GitHub REST API, handling pagination automatically.
func fetchAllTags(token string) ([]string, error) {
	var names []string
	client := &http.Client{Timeout: 30 * time.Second}

	for page := 1; ; page++ {
		url := fmt.Sprintf("%s?per_page=100&page=%d", githubTagsURL, page)

		req, err := http.NewRequest(http.MethodGet, url, nil)
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

// filterStableTags returns only tags that represent a stable patch release.
func filterStableTags(tags []string) []string {
	var stable []string
	for _, tag := range tags {
		if stableTagRe.MatchString(tag) {
			stable = append(stable, tag)
		}
	}
	return stable
}

// groupByMinor groups patch version strings by their minor component.
// Input tags are expected to have a leading "v"; output keys do not.
func groupByMinor(tags []string) map[string][]string {
	groups := make(map[string][]string)
	for _, tag := range tags {
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

// topMinors returns the n highest minor versions from the map, sorted
// numerically descending.
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

// minorGreater reports whether minor version a is greater than b ("MAJOR.MINOR").
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
func sortPatchesDesc(patches []string) {
	sort.Slice(patches, func(i, j int) bool {
		return patchGreater(patches[i], patches[j])
	})
}

// patchGreater reports whether patch version a is greater than b ("MAJOR.MINOR.PATCH").
func patchGreater(a, b string) bool {
	aParts := strings.SplitN(a, ".", 3)
	bParts := strings.SplitN(b, ".", 3)
	if len(aParts) != 3 || len(bParts) != 3 {
		return a > b
	}
	for i := 0; i < 3; i++ {
		av, _ := strconv.Atoi(aParts[i])
		bv, _ := strconv.Atoi(bParts[i])
		if av != bv {
			return av > bv
		}
	}
	return false
}
