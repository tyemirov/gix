package workflow

import (
	"embed"
	"sort"
	"strings"

	"github.com/temirov/gix/internal/workflow"
)

//go:embed presets/*.yaml
var embeddedWorkflowPresets embed.FS

// PresetMetadata describes an embedded workflow.
type PresetMetadata struct {
	Name        string
	Description string
}

// PresetCatalog loads embedded workflow presets.
type PresetCatalog interface {
	List() []PresetMetadata
	Load(name string) (workflow.Configuration, bool, error)
}

type presetDefinition struct {
	Name        string
	Description string
	FileName    string
}

var embeddedPresetDefinitions = []presetDefinition{
	{
		Name:        "license",
		Description: "Distribute a license file across repositories using tasks apply.",
		FileName:    "presets/license.yaml",
	},
	{
		Name:        "namespace",
		Description: "Rewrite Go module namespaces using tasks apply.",
		FileName:    "presets/namespace.yaml",
	},
	{
		Name:        "folder-rename",
		Description: "Normalize repository folders so directories match canonical GitHub names.",
		FileName:    "presets/folder-rename.yaml",
	},
	{
		Name:        "remote-update-to-canonical",
		Description: "Update origin remotes to match canonical GitHub repositories.",
		FileName:    "presets/remote-update-to-canonical.yaml",
	},
	{
		Name:        "remote-update-protocol",
		Description: "Convert origin remotes between git/ssh/https protocols.",
		FileName:    "presets/remote-update-protocol.yaml",
	},
	{
		Name:        "history-remove",
		Description: "Purge repository history paths via git-filter-repo.",
		FileName:    "presets/history-remove.yaml",
	},
}

type embeddedPresetCatalog struct {
	files       embed.FS
	definitions []presetDefinition
}

// NewEmbeddedPresetCatalog constructs a PresetCatalog backed by embedded YAML definitions.
func NewEmbeddedPresetCatalog() PresetCatalog {
	return &embeddedPresetCatalog{
		files:       embeddedWorkflowPresets,
		definitions: embeddedPresetDefinitions,
	}
}

func (catalog *embeddedPresetCatalog) List() []PresetMetadata {
	if catalog == nil || len(catalog.definitions) == 0 {
		return nil
	}

	metadata := make([]PresetMetadata, 0, len(catalog.definitions))
	for index := range catalog.definitions {
		definition := catalog.definitions[index]
		metadata = append(metadata, PresetMetadata{
			Name:        definition.Name,
			Description: definition.Description,
		})
	}

	sort.Slice(metadata, func(firstIndex int, secondIndex int) bool {
		return metadata[firstIndex].Name < metadata[secondIndex].Name
	})

	return metadata
}

func (catalog *embeddedPresetCatalog) Load(name string) (workflow.Configuration, bool, error) {
	if catalog == nil {
		return workflow.Configuration{}, false, nil
	}

	for index := range catalog.definitions {
		definition := catalog.definitions[index]
		if !strings.EqualFold(name, definition.Name) {
			continue
		}

		content, readError := catalog.files.ReadFile(definition.FileName)
		if readError != nil {
			return workflow.Configuration{}, true, readError
		}

		configuration, parseError := workflow.ParseConfiguration(content)
		if parseError != nil {
			return workflow.Configuration{}, true, parseError
		}

		return configuration, true, nil
	}

	return workflow.Configuration{}, false, nil
}
