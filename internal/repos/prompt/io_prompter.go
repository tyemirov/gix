package prompt

import (
	"bufio"
	"io"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	affirmativeShortResponseConstant = "y"
	affirmativeLongResponseConstant  = "yes"
	applyAllShortResponseConstant    = "a"
	applyAllLongResponseConstant     = "all"
)

// IOConfirmationPrompter reads confirmation responses from an io.Reader.
type IOConfirmationPrompter struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewIOConfirmationPrompter constructs a prompter from the provided reader and writer.
func NewIOConfirmationPrompter(input io.Reader, output io.Writer) *IOConfirmationPrompter {
	return &IOConfirmationPrompter{reader: bufio.NewReader(input), writer: output}
}

// Confirm writes the prompt and interprets affirmative responses including "all" to apply globally.
func (prompter *IOConfirmationPrompter) Confirm(prompt string) (shared.ConfirmationResult, error) {
	if prompter.writer != nil {
		if _, writeError := io.WriteString(prompter.writer, prompt); writeError != nil {
			return shared.ConfirmationResult{}, writeError
		}
	}

	response, readError := prompter.reader.ReadString('\n')
	if readError != nil && readError != io.EOF {
		return shared.ConfirmationResult{}, readError
	}

	normalizedResponse := strings.TrimSpace(strings.ToLower(response))
	switch normalizedResponse {
	case affirmativeShortResponseConstant, affirmativeLongResponseConstant:
		return shared.ConfirmationResult{Confirmed: true}, nil
	case applyAllShortResponseConstant, applyAllLongResponseConstant:
		return shared.ConfirmationResult{Confirmed: true, ApplyToAll: true}, nil
	default:
		return shared.ConfirmationResult{}, nil
	}
}
