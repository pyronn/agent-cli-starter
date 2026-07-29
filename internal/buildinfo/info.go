package buildinfo

// Info contains build metadata injected through -ldflags.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}
