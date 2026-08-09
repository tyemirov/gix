// Package releaseversion owns the canonical release scheme and successor rules.
package releaseversion

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

const (
	ContractVersion     = "mprlab.version-decision/v1"
	ConfigurationPath   = ".mprlab/release.yml"
	ConfigurationSchema = 1
)

// Scheme is one configured release version scheme.
type Scheme string

const (
	SchemeSemVer Scheme = "semver"
	SchemeCalVer Scheme = "calver"
)

// Bump is one closed SemVer successor level.
type Bump string

const (
	BumpPatch Bump = "patch"
	BumpMinor Bump = "minor"
	BumpMajor Bump = "major"
)

// Configuration is the source-controlled release version contract.
type Configuration struct {
	SchemaVersion int                     `yaml:"schema_version"`
	Scheme        Scheme                  `yaml:"scheme"`
	GoInstall     *GoInstallConfiguration `yaml:"go_install,omitempty"`
}

// GoInstallConfiguration defines one root-module installation channel.
type GoInstallConfiguration struct {
	ModulePath         string `yaml:"module_path"`
	ProductVersionFile string `yaml:"product_version_file"`
}

var (
	semverPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	calverPattern = regexp.MustCompile(`^(1[0-9]|[2-9][0-9])\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,5})$`)
)

// ParseConfiguration decodes the strict repository release contract.
func ParseConfiguration(data []byte) (Configuration, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	configuration := Configuration{}
	if decodeError := decoder.Decode(&configuration); decodeError != nil {
		return Configuration{}, fmt.Errorf("decode release configuration: %w", decodeError)
	}
	var trailing any
	if trailingError := decoder.Decode(&trailing); !errors.Is(trailingError, io.EOF) {
		if trailingError != nil {
			return Configuration{}, fmt.Errorf("decode trailing release configuration: %w", trailingError)
		}
		return Configuration{}, errors.New("release configuration contains multiple YAML documents")
	}
	if configuration.SchemaVersion != ConfigurationSchema {
		return Configuration{}, fmt.Errorf("release configuration schema_version must be %d", ConfigurationSchema)
	}
	if configuration.Scheme != SchemeSemVer && configuration.Scheme != SchemeCalVer {
		return Configuration{}, fmt.Errorf("release configuration scheme must be %q or %q", SchemeSemVer, SchemeCalVer)
	}
	if configuration.GoInstall != nil {
		configuration.GoInstall.ModulePath = strings.TrimSpace(configuration.GoInstall.ModulePath)
		if moduleError := module.CheckPath(configuration.GoInstall.ModulePath); moduleError != nil {
			return Configuration{}, fmt.Errorf("release configuration go_install.module_path is invalid: %w", moduleError)
		}
		configuration.GoInstall.ProductVersionFile = strings.TrimSpace(configuration.GoInstall.ProductVersionFile)
		if configuration.GoInstall.ProductVersionFile == "" ||
			path.Clean(configuration.GoInstall.ProductVersionFile) != configuration.GoInstall.ProductVersionFile ||
			strings.Contains(configuration.GoInstall.ProductVersionFile, "\\") ||
			strings.HasPrefix(configuration.GoInstall.ProductVersionFile, "/") ||
			configuration.GoInstall.ProductVersionFile == ".." ||
			strings.HasPrefix(configuration.GoInstall.ProductVersionFile, "../") {
			return Configuration{}, errors.New("release configuration go_install.product_version_file must be a canonical repository-relative path")
		}
	}
	return configuration, nil
}

// Valid reports whether a bump belongs to the closed SemVer set.
func (bump Bump) Valid() bool {
	return bump == BumpPatch || bump == BumpMinor || bump == BumpMajor
}

// Rank returns the precedence of one SemVer bump.
func (bump Bump) Rank() int {
	switch bump {
	case BumpMajor:
		return 3
	case BumpMinor:
		return 2
	case BumpPatch:
		return 1
	default:
		return 0
	}
}

// MaximumBump returns the higher of two valid SemVer levels.
func MaximumBump(first Bump, second Bump) Bump {
	if first.Rank() >= second.Rank() {
		return first
	}
	return second
}

// LatestSemVer returns the greatest stable canonical SemVer tag.
func LatestSemVer(tags []string) string {
	versions := make([]semverValue, 0, len(tags))
	for _, tag := range tags {
		if parsed, parseError := parseSemVer(tag); parseError == nil {
			versions = append(versions, parsed)
		}
	}
	if len(versions) == 0 {
		return ""
	}
	sort.Slice(versions, func(first int, second int) bool {
		return versions[first].compare(versions[second]) > 0
	})
	return versions[0].raw
}

// NextSemVer applies one canonical successor operation.
func NextSemVer(previous string, bump Bump) (string, error) {
	if !bump.Valid() {
		return "", fmt.Errorf("SemVer bump must be patch, minor, or major: %q", bump)
	}
	parsed, parseError := parseSemVer(previous)
	if parseError != nil {
		return "", parseError
	}
	one := big.NewInt(1)
	switch bump {
	case BumpMajor:
		parsed.major.Add(parsed.major, one)
		parsed.minor.SetInt64(0)
		parsed.patch.SetInt64(0)
	case BumpMinor:
		parsed.minor.Add(parsed.minor, one)
		parsed.patch.SetInt64(0)
	case BumpPatch:
		parsed.patch.Add(parsed.patch, one)
	}
	return fmt.Sprintf("v%s.%s.%s", parsed.major.String(), parsed.minor.String(), parsed.patch.String()), nil
}

// NextGoInstallVersion returns the next patch on the root module's v1
// transport line.
func NextGoInstallVersion(tags []string) (string, error) {
	latest := semverValue{}
	found := false
	for _, tag := range tags {
		parsed, parseError := parseSemVer(tag)
		if parseError != nil || parsed.major.Cmp(big.NewInt(1)) != 0 {
			continue
		}
		if !found || parsed.compare(latest) > 0 {
			latest = parsed
			found = true
		}
	}
	if !found {
		return "v1.0.0", nil
	}
	return NextSemVer(latest.raw, BumpPatch)
}

// LatestCalVer returns the greatest canonical CalVer tag.
func LatestCalVer(tags []string) string {
	values := make([]calverValue, 0, len(tags))
	for _, tag := range tags {
		if parsed, parseError := parseCalVer(tag); parseError == nil {
			values = append(values, parsed)
		}
	}
	if len(values) == 0 {
		return ""
	}
	sort.Slice(values, func(first int, second int) bool {
		return values[first].timestamp.After(values[second].timestamp)
	})
	return values[0].raw
}

// NextCalVer returns the canonical UTC CalVer for a release timestamp.
func NextCalVer(previous string, releaseTime time.Time) (string, error) {
	utc := releaseTime.UTC()
	if utc.Year() < 2010 || utc.Year() > 2099 {
		return "", fmt.Errorf("CalVer release year must be between 2010 and 2099: %d", utc.Year())
	}
	candidate := fmt.Sprintf("%d.%d.%d", utc.Year()%100, int(utc.Month())*100+utc.Day(), utc.Hour()*10000+utc.Minute()*100+utc.Second())
	parsedCandidate, candidateError := parseCalVer(candidate)
	if candidateError != nil {
		return "", candidateError
	}
	if strings.TrimSpace(previous) == "" {
		return candidate, nil
	}
	parsedPrevious, previousError := parseCalVer(previous)
	if previousError != nil {
		return "", previousError
	}
	if !parsedCandidate.timestamp.After(parsedPrevious.timestamp) {
		return "", fmt.Errorf("CalVer release timestamp must be later than %s", previous)
	}
	return candidate, nil
}

type semverValue struct {
	raw   string
	major *big.Int
	minor *big.Int
	patch *big.Int
}

func parseSemVer(value string) (semverValue, error) {
	match := semverPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return semverValue{}, fmt.Errorf("stable SemVer tag must use vMAJOR.MINOR.PATCH: %q", value)
	}
	return semverValue{
		raw:   match[0],
		major: parseBigInteger(match[1]),
		minor: parseBigInteger(match[2]),
		patch: parseBigInteger(match[3]),
	}, nil
}

func parseBigInteger(value string) *big.Int {
	parsed, _ := new(big.Int).SetString(value, 10)
	return parsed
}

func (value semverValue) compare(other semverValue) int {
	if result := value.major.Cmp(other.major); result != 0 {
		return result
	}
	if result := value.minor.Cmp(other.minor); result != 0 {
		return result
	}
	return value.patch.Cmp(other.patch)
}

type calverValue struct {
	raw       string
	timestamp time.Time
}

func parseCalVer(value string) (calverValue, error) {
	trimmed := strings.TrimSpace(value)
	match := calverPattern.FindStringSubmatch(trimmed)
	if match == nil {
		return calverValue{}, fmt.Errorf("CalVer tag must use YY.MDD.HHMMSS: %q", value)
	}
	year := parseSmallInteger(match[1])
	monthDay := parseSmallInteger(match[2])
	timeValue := fmt.Sprintf("%06d", parseSmallInteger(match[3]))
	month := monthDay / 100
	day := monthDay % 100
	hour := parseSmallInteger(timeValue[:2])
	minute := parseSmallInteger(timeValue[2:4])
	second := parseSmallInteger(timeValue[4:])
	parsed := time.Date(2000+year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	if parsed.Year() != 2000+year || int(parsed.Month()) != month || parsed.Day() != day || parsed.Hour() != hour || parsed.Minute() != minute || parsed.Second() != second {
		return calverValue{}, fmt.Errorf("CalVer tag contains an invalid UTC timestamp: %q", value)
	}
	return calverValue{raw: trimmed, timestamp: parsed}, nil
}

func parseSmallInteger(value string) int {
	result := 0
	for _, character := range value {
		result = result*10 + int(character-'0')
	}
	return result
}
