package cmd

import (
	"strings"

	"github.com/coreos/go-semver/semver"
)

// A VersionInfo contains a version.
type VersionInfo struct {
	Version string
	Commit  string
	BuiltBy string
}

func versionString(versionInfo VersionInfo) string {
	var versionElems []string
	if versionInfo.Version != "" {
		version, err := semver.NewVersion(strings.TrimPrefix(versionInfo.Version, "v"))
		if err != nil {
			return versionInfo.Version
		}

		versionElems = append(versionElems, "v"+version.String())
	} else {
		versionElems = append(versionElems, "dev")
	}
	if versionInfo.Commit != "" {
		versionElems = append(versionElems, "commit "+versionInfo.Commit)
	}
	if versionInfo.BuiltBy != "" {
		versionElems = append(versionElems, "built by "+versionInfo.BuiltBy)
	}
	return strings.Join(versionElems, ", ")
}
