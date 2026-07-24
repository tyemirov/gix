package cli

import _ "embed"

//go:embed default_config.yml
var embeddedDefaultConfigurationContent []byte

// EmbeddedDefaultConfiguration returns the embedded default configuration data and type identifier.
func EmbeddedDefaultConfiguration() ([]byte, string) {
	duplicatedContent := make([]byte, len(embeddedDefaultConfigurationContent))
	copy(duplicatedContent, embeddedDefaultConfigurationContent)
	return duplicatedContent, configurationTypeConstant
}
