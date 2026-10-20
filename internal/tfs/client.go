package tfs

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
)

// Client handles executing tf commands.
type Client struct {
	LogFunc func(string)
}

// NewClient returns a new Client.
func NewClient() *Client {
	return &Client{}
}

// execute runs a tf command and returns its output.
func (c *Client) execute(args ...string) (string, error) {
	if c.LogFunc != nil {
		c.LogFunc("tf " + strings.Join(args, " "))
	}
	cmd := exec.Command("tf", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// GetStatus returns the output of `tf status` for the current folder only,
// so pending changes from other mappings in the same workspace don't show up.
func (c *Client) GetStatus() (string, error) {
	return c.execute("status", ".", "/recursive")
}

// GetBranches returns the output of `tf branches .`.
func (c *Client) GetBranches() (string, error) {
	return c.execute("branches", ".")
}

// HistoryPageSize is how many changesets GetHistory returns per call.
const HistoryPageSize = 50

// GetHistory returns the output of `tf history`, one page of HistoryPageSize changesets.
// beforeID > 0 returns the page of changesets older than beforeID (for "load more").
func (c *Client) GetHistory(author string, beforeID int) (string, error) {
	args := []string{"history", ".", "/r", "/noprompt", "/stopafter:" + strconv.Itoa(HistoryPageSize)}
	if author != "" {
		args = append(args, "/user:"+author)
	}
	if beforeID > 0 {
		args = append(args, "/version:~C"+strconv.Itoa(beforeID-1))
	}
	return c.execute(args...)
}

// GetChangesetDetail returns the details of a specific changeset.
func (c *Client) GetChangesetDetail(changesetID string) (string, error) {
	return c.execute("changeset", changesetID, "/noprompt")
}

// GetChangesetFileDiff returns the unified diff of one server item between
// the changeset before changesetID and changesetID itself.
func (c *Client) GetChangesetFileDiff(serverPath string, changesetID int) (string, error) {
	versionSpec := "C" + strconv.Itoa(changesetID-1) + "~C" + strconv.Itoa(changesetID)
	return c.execute("diff", serverPath, "/version:"+versionSpec, "/format:Unified")
}

// ViewFile returns the content of a server item at a specific changeset.
func (c *Client) ViewFile(serverPath string, changesetID int) (string, error) {
	return c.execute("view", serverPath, "/version:C"+strconv.Itoa(changesetID), "/console")
}

// GetShelvesets returns the output of `tf shelvesets`.
func (c *Client) GetShelvesets() (string, error) {
	return c.execute("shelvesets")
}

// GetDiff returns the unified diff for a specific file.
func (c *Client) GetDiff(filename string) (string, error) {
	return c.execute("diff", filename, "/format:Unified")
}

// GetWorkspace returns the current workspace info.
func (c *Client) GetWorkspace() (string, error) {
	return c.execute("workfold")
}

// GetLatest returns the output of `tf get` for the given files/folders.
func (c *Client) GetLatest(paths []string) (string, error) {
	args := []string{"get"}
	args = append(args, paths...)
	// Use /noprompt so it doesn't block waiting for input in case of conflicts
	args = append(args, "/recursive", "/noprompt")
	return c.execute(args...)
}

// ResolveConflicts resolves conflicts for a specific path by taking the server's version or keeping yours.
func (c *Client) ResolveConflicts(path string, takeServer bool) (string, error) {
	resolution := "/auto:KeepYours"
	if takeServer {
		resolution = "/auto:TakeTheirs"
	}
	return c.execute("resolve", path, "/recursive", resolution, "/noprompt")
}

// Checkin executes a checkin for the specified files with the given comment.
func (c *Client) Checkin(files []string, comment string) (string, error) {
	args := []string{"checkin"}
	args = append(args, files...)
	args = append(args, "/comment:"+comment, "/noprompt")
	return c.execute(args...)
}

// Conflict represents a file conflict with its path and reason.
type Conflict struct {
	Path   string
	Reason string
}

// GetConflicts returns a list of files with conflicts in the specified path.
func (c *Client) GetConflicts(path string) ([]Conflict, error) {
	out, err := c.execute("resolve", path, "/recursive", "/preview", "/noprompt")
	if err != nil {
		// tf might return exit code 1 if there are conflicts, so we don't return early if out != ""
		if out == "" {
			return nil, err
		}
	}

	var conflicts []Conflict
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && (strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "/")) {
			filePath := trimmed
			reason := ""
			idx := strings.Index(trimmed, ": ")
			if idx != -1 {
				part1 := strings.TrimSpace(trimmed[:idx])
				part2 := strings.TrimSpace(trimmed[idx+2:])
				
				if strings.Contains(part1, "\\") || strings.Contains(part1, "/") {
					filePath = part1
					reason = part2
				} else if strings.Contains(part2, "\\") || strings.Contains(part2, "/") {
					filePath = part2
					reason = part1
				}
			}
			conflicts = append(conflicts, Conflict{Path: filePath, Reason: reason})
		}
	}
	return conflicts, nil
}
