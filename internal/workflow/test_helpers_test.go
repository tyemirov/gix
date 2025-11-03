package workflow

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func parseStructuredEvents(output string) []map[string]string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var events []map[string]string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 || strings.HasPrefix(line, "Summary:") || strings.HasPrefix(line, "-- ") {
			continue
		}

		parts := strings.Split(line, " | ")
		machinePart := parts[len(parts)-1]

		fields := strings.Fields(machinePart)
		if len(fields) == 0 {
			continue
		}

		event := make(map[string]string, len(fields))
		for _, field := range fields {
			keyValue := strings.SplitN(field, "=", 2)
			if len(keyValue) != 2 {
				continue
			}
			event[keyValue[0]] = keyValue[1]
		}

		if len(event) > 0 {
			events = append(events, event)
		}
	}

	return events
}

func requireEventByCode(t *testing.T, events []map[string]string, code string) map[string]string {
	for _, event := range events {
		if event["event"] == code {
			return event
		}
	}
	require.Failf(t, "event not found", "expected event code %s in %+v", code, events)
	return nil
}
