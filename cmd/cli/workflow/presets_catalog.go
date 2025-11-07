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
		Description: "Generate a license audit report across repositories.",
		FileName:    "presets/license.yaml",
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
