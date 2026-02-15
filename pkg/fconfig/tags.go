package fconfig

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func ResolveRepoIndex(catalog *Catalog, selector string) (int, error) {
	if catalog == nil {
		return -1, errors.New("nil catalog")
	}
	if selector == "" {
		return -1, errors.New("empty repository selector")
	}

	for index, repo := range catalog.Repos {
		if repo.ID == selector {
			return index, nil
		}
	}

	normalizedSelector := filepath.Clean(selector)
	for index, repo := range catalog.Repos {
		for _, location := range repo.Locations {
			if filepath.Clean(location.Path) == normalizedSelector {
				return index, nil
			}
		}
	}

	return -1, fmt.Errorf("repository %q not found in catalog", selector)
}

func AddTags(catalog *Catalog, selector string, tags []string) error {
	index, err := ResolveRepoIndex(catalog, selector)
	if err != nil {
		return err
	}

	catalog.Repos[index].Tags = normalizeTags(append(catalog.Repos[index].Tags, tags...))

	return nil
}

func RemoveTags(catalog *Catalog, selector string, tags []string) error {
	index, err := ResolveRepoIndex(catalog, selector)
	if err != nil {
		return err
	}

	normalizedToRemove := normalizeTags(tags)
	toRemoveSet := make(map[string]struct{}, len(normalizedToRemove))
	for _, tag := range normalizedToRemove {
		toRemoveSet[tag] = struct{}{}
	}

	filtered := make([]string, 0, len(catalog.Repos[index].Tags))
	for _, tag := range catalog.Repos[index].Tags {
		normalizedTag := strings.TrimSpace(tag)
		if normalizedTag == "" {
			continue
		}
		if _, ok := toRemoveSet[normalizedTag]; ok {
			continue
		}

		filtered = append(filtered, normalizedTag)
	}

	catalog.Repos[index].Tags = normalizeTags(filtered)
	return nil
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}

		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	sort.Strings(out)
	return out
}
