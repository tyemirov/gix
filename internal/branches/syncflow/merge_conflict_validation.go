package syncflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validate the assembled file at the model-output boundary, before staging.
// Syntax validation does not establish that the merged behavior is correct.
func validateAssembledMergeFile(path, content string) error {
	var validationErr error
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		_, validationErr = parser.ParseFile(token.NewFileSet(), path, content, parser.AllErrors)
	case ".json":
		if !json.Valid([]byte(content)) {
			validationErr = errors.New("invalid JSON document")
		}
	case ".yaml", ".yml":
		decoder := yaml.NewDecoder(strings.NewReader(content))
		for {
			var document yaml.Node
			err := decoder.Decode(&document)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				validationErr = err
				break
			}
		}
	}
	if validationErr != nil {
		return fmt.Errorf("validate assembled merge file %s: %w", path, validationErr)
	}
	if filepath.Base(path) == "ISSUES.md" || filepath.Base(path) == "ARCHIVE.md" {
		seen := map[string]bool{}
		for _, line := range mergeConflictLines(content) {
			header := mergeConflictIssueHeader.FindStringSubmatch(line)
			if header == nil {
				continue
			}
			if seen[header[1]] {
				return fmt.Errorf("validate assembled merge file %s: duplicate issue identifier %s", path, header[1])
			}
			seen[header[1]] = true
		}
	}
	return nil
}
