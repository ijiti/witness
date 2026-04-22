package claudelog

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// AgentPersona represents a parsed agent persona from ~/.claude/agents/*.md.
type AgentPersona struct {
	Name        string
	Nickname    string
	Description string
	Model       string
	Color       string // CSS-friendly color name: orange, cyan, blue, etc.
}

// ParseAgentPersonas reads all *.md files from the agents directory and
// extracts YAML frontmatter fields. Returns a map keyed by filename stem
// (e.g., "backend-generalist").
func ParseAgentPersonas(dir string) (map[string]*AgentPersona, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*AgentPersona)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		persona, err := parsePersonaFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if persona.Name == "" {
			persona.Name = name
		}
		result[name] = persona
	}
	return result, nil
}

// parsePersonaFile extracts YAML frontmatter fields from an agent .md file.
// Parses between --- delimiters manually to avoid importing a YAML library.
func parsePersonaFile(path string) (*AgentPersona, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	p := &AgentPersona{}

	// Expect first line to be "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return p, nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		switch key {
		case "name":
			p.Name = val
		case "nickname":
			p.Nickname = val
		case "description":
			p.Description = val
		case "model":
			p.Model = val
		case "color":
			p.Color = val
		}
	}
	return p, nil
}
