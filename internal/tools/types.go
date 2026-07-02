package tools

// Finding is a security issue detected by a static analysis tool.
type Finding struct {
	Tool        string `json:"tool"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
	Line        int    `json:"line,omitempty"`
}
