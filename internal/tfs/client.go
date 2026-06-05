package tfs

import (
	"bytes"
	"os/exec"
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

// GetStatus returns the output of `tf status`.
func (c *Client) GetStatus() (string, error) {
	return c.execute("status")
}

// GetBranches returns the output of `tf branches .`.
func (c *Client) GetBranches() (string, error) {
	return c.execute("branches", ".")
}

// GetHistory returns the output of `tf history`.
func (c *Client) GetHistory() (string, error) {
	return c.execute("history", ".", "/r", "/noprompt", "/stopafter:50")
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
