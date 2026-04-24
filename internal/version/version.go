package version

import "fmt"

var (
	Version = "0.0.0-dev"
	Commit  = "none"
	Date    = "unknown"
)

type Info struct {
	Version string
	Commit  string
	Date    string
}

func Get() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}

func String() string {
	info := Get()
	return fmt.Sprintf("%s (%s, %s)", info.Version, info.Commit, info.Date)
}
