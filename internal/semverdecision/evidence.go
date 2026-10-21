package semverdecision

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// EvidenceFragment records an exact part of one evidence source.
type EvidenceFragment struct {
	Kind        string `json:"kind"`
	OffsetRunes int    `json:"offset_runes"`
	Content     string `json:"content"`
	Context     string `json:"context,omitempty"`
}

// EvidencePacket records the initial bounded model input.
type EvidencePacket struct {
	ID        string             `json:"id"`
	SHA256    string             `json:"sha256"`
	Content   string             `json:"content"`
	Fragments []EvidenceFragment `json:"fragments"`
}

// EvidenceCoverage records the complete size and digest of one evidence source.
type EvidenceCoverage struct {
	Kind         string `json:"kind"`
	SHA256       string `json:"sha256"`
	RuneCount    int    `json:"rune_count"`
	CoveredRunes int    `json:"covered_runes"`
}

type evidenceSource struct{ kind, content string }

func buildEvidencePackets(evidence repositoryEvidence, options Options) ([]EvidencePacket, []EvidenceCoverage, error) {
	boundary := options.BoundaryLabel
	if boundary == "" {
		boundary = options.SinceReference
	}
	prefix := fmt.Sprintf("Release range: %s..%s\nDeterministic minimum: patch\n", boundary, options.SourceReference)
	sources := []evidenceSource{
		{"Commit messages", evidence.commitLog},
		{"Diff summary", evidence.diffSummary},
		{"Changelog changes in the release range", evidence.changelogChanges},
		{"Diff excerpt", evidence.diffExcerpt},
	}
	capacity := evidencePacketRuneLimit - len([]rune(prefix)) - 200
	if capacity < 1000 {
		return nil, nil, fmt.Errorf("semver decision release labels exceed evidence packet limit")
	}
	packets := []EvidencePacket{}
	coverage := []EvidenceCoverage{}
	current := EvidencePacket{}
	body := ""
	flush := func() {
		if len(current.Fragments) == 0 {
			return
		}
		current.ID = fmt.Sprintf("packet-%04d", len(packets)+1)
		current.Content = prefix + "Evidence packet: " + current.ID + "\n" + body + "\nDecide the required SemVer bump for this evidence packet now."
		current.SHA256 = textDigest(current.Content)
		packets = append(packets, current)
		current, body = EvidencePacket{}, ""
	}
	for _, source := range sources {
		runes := []rune(source.content)
		coverage = append(coverage, EvidenceCoverage{Kind: source.kind, SHA256: textDigest(source.content), RuneCount: len(runes), CoveredRunes: 0})
		offset := 0
		fileContext, hunkContext := "", ""
		for offset < len(runes) {
			if offset == 0 || runes[offset-1] == '\n' {
				prefix := string(runes[offset:min(offset+16, len(runes))])
				if strings.HasPrefix(prefix, "diff --git ") || strings.HasPrefix(prefix, "@@ ") {
					lineEnd := offset
					for lineEnd < len(runes) && runes[lineEnd] != '\n' {
						lineEnd++
					}
					firstLine := string(runes[offset:lineEnd])
					if strings.HasPrefix(firstLine, "diff --git ") {
						fileContext, hunkContext = firstLine, ""
					} else {
						hunkContext = firstLine
					}
				}
			}
			context := strings.TrimSpace(fileContext + "\n" + hunkContext)
			label := fmt.Sprintf("\n%s (source rune offset %d):\n", source.kind, offset)
			if context != "" {
				label += "Fragment context:\n" + context + "\nExact fragment:\n"
			}
			available := capacity - len([]rune(body+label))
			if available < 512 && len(current.Fragments) > 0 {
				flush()
				continue
			}
			if available <= 0 {
				flush()
				return packets, coverage, fmt.Errorf("semver decision fragment metadata exceeds evidence packet limit")
			}
			end := min(offset+available, len(runes))
			// Prefer complete lines and preserve each file or hunk boundary.
			lastLineEnd := offset
			for position := offset; position < end; position++ {
				if position == offset || runes[position-1] == '\n' {
					remainder := string(runes[position:min(position+16, end)])
					if position > offset && (strings.HasPrefix(remainder, "diff --git ") || strings.HasPrefix(remainder, "@@ ")) {
						end = position
						break
					}
				}
				if runes[position] == '\n' {
					lastLineEnd = position + 1
				}
			}
			if end < len(runes) && lastLineEnd > offset && lastLineEnd < end {
				end = lastLineEnd
			}
			content := string(runes[offset:end])
			fragment := EvidenceFragment{Kind: source.kind, OffsetRunes: offset, Content: content, Context: context}
			current.Fragments = append(current.Fragments, fragment)
			coverage[len(coverage)-1].CoveredRunes += len([]rune(content))
			body += label + content
			for _, line := range strings.Split(content, "\n") {
				if strings.HasPrefix(line, "diff --git ") {
					fileContext, hunkContext = line, ""
				}
				if strings.HasPrefix(line, "@@ ") {
					hunkContext = line
				}
			}
			offset = end
		}
	}
	flush()
	for _, packet := range packets {
		if len([]rune(packet.Content)) > evidencePacketRuneLimit {
			return packets, coverage, fmt.Errorf("semver decision packet %s exceeds %d rune limit", packet.ID, evidencePacketRuneLimit)
		}
	}
	return packets, coverage, nil
}

func textDigest(content string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(content))) }
