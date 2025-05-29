package buildinfo

import (
	"crypto/md5"
	"fmt"
	"os"
	"time"
)

var (
	// These will be set at build time via ldflags or at runtime
	BuildTime    = ""
	BuildHash    = ""
	BuildVersion = "dev"
)

// CacheVersion returns a cache-busting version string
func CacheVersion() string {
	// In development mode, use file modification time for live updates
	if BuildVersion == "dev" || BuildVersion == "" {
		if stat, err := os.Stat("static/output.css"); err == nil {
			return fmt.Sprintf("%d", stat.ModTime().Unix())
		}
	}
	
	if BuildHash != "" {
		if len(BuildHash) >= 8 {
			return BuildHash[:8] // Use first 8 chars of git hash
		}
		return BuildHash // Use full hash if shorter than 8 chars
	}
	
	if BuildTime != "" {
		return BuildTime
	}
	
	// Fallback: use file modification time of the CSS file
	if stat, err := os.Stat("static/output.css"); err == nil {
		return fmt.Sprintf("%d", stat.ModTime().Unix())
	}
	
	// Final fallback: use current time hash
	return fmt.Sprintf("%x", md5.Sum([]byte(time.Now().String())))[:8]
}

// GetBuildInfo returns build information
func GetBuildInfo() map[string]string {
	return map[string]string{
		"version":      BuildVersion,
		"buildTime":    BuildTime,
		"buildHash":    BuildHash,
		"cacheVersion": CacheVersion(),
	}
}

// GetDisplayVersion returns a human-readable version string
func GetDisplayVersion() string {
	if BuildVersion == "dev" {
		return "Development"
	}
	
	if BuildHash != "" {
		return fmt.Sprintf("prod (%s)", BuildHash)
	}
	
	if BuildTime != "" {
		// Format timestamp as readable date
		if len(BuildTime) == 14 { // YYYYMMDDHHMMSS format
			if t, err := time.Parse("20060102150405", BuildTime); err == nil {
				return fmt.Sprintf("prod (%s)", t.Format("Jan 2 15:04"))
			}
		}
		return fmt.Sprintf("prod (%s)", BuildTime)
	}
	
	return "prod"
}

// GetBuildDate returns a formatted build date
func GetBuildDate() string {
	if BuildTime == "" {
		return "Unknown"
	}
	
	// Format timestamp as readable date
	if len(BuildTime) == 14 { // YYYYMMDDHHMMSS format
		if t, err := time.Parse("20060102150405", BuildTime); err == nil {
			return t.Format("Jan 2, 2006 at 15:04 UTC")
		}
	}
	
	return BuildTime
}