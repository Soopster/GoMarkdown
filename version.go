package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

func appVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
			return v
		}
	}
	if v := strings.TrimSpace(version); v != "" {
		return v
	}
	return "dev"
}

func versionDetails() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		version := appVersion()
		var revision, modified, vcsTime string
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				revision = setting.Value
			case "vcs.modified":
				modified = setting.Value
			case "vcs.time":
				vcsTime = setting.Value
			}
		}

		var b strings.Builder
		b.WriteString("markdownviewer ")
		b.WriteString(version)
		if revision != "" {
			b.WriteString("\nrevision: ")
			b.WriteString(revision)
		}
		if vcsTime != "" {
			b.WriteString("\nvcs time: ")
			b.WriteString(vcsTime)
		}
		if modified == "true" {
			b.WriteString("\nmodified: true")
		}
		if info.GoVersion != "" {
			b.WriteString("\ngo: ")
			b.WriteString(info.GoVersion)
		}
		return b.String()
	}
	return fmt.Sprintf("markdownviewer %s", appVersion())
}
