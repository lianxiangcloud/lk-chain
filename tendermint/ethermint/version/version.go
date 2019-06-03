package version

const (
	// Major version component of the current release
	Major = 0
	// Minor version component of the current release
	Minor = 5
	// Fix version component of the current release
	Fix = 3
)


var (
	// Version is the full version string
	Version = "0.5.3"
	// GitCommit is set with --ldflags "-X main.gitCommit=$(git rev-parse HEAD)"
	GitCommit string
)

func init() {
	if GitCommit != "" && len(GitCommit) >= 8 {
		Version += "-" + GitCommit[:8]
	}
}
