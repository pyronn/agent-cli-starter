package update

import (
	"strconv"
	"strings"
)

// version is a parsed semantic version. Build metadata is ignored during
// comparison, while prerelease identifiers are ordered per the SemVer rules.
type version struct {
	major int
	minor int
	patch int
	pre   []string
}

func parseVersion(raw string) (version, bool) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return version{}, false
	}
	if index := strings.IndexByte(value, '+'); index >= 0 {
		value = value[:index]
	}
	var pre []string
	if index := strings.IndexByte(value, '-'); index >= 0 {
		pre = strings.Split(value[index+1:], ".")
		value = value[:index]
		for _, identifier := range pre {
			if identifier == "" {
				return version{}, false
			}
		}
	}
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return version{}, false
	}
	numbers := [3]int{}
	for index, part := range parts {
		if !isDigits(part) {
			return version{}, false
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return version{}, false
		}
		numbers[index] = number
	}
	return version{major: numbers[0], minor: numbers[1], patch: numbers[2], pre: pre}, true
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// validTag reports whether value is a release tag such as v1.2.3 or v1.2.3-rc.1.
func validTag(value string) bool {
	if !strings.HasPrefix(value, "v") {
		return false
	}
	_, ok := parseVersion(value)
	return ok
}

func (v version) compare(other version) int {
	if v.major != other.major {
		return sign(v.major - other.major)
	}
	if v.minor != other.minor {
		return sign(v.minor - other.minor)
	}
	if v.patch != other.patch {
		return sign(v.patch - other.patch)
	}
	return comparePrerelease(v.pre, other.pre)
}

// comparePrerelease orders prerelease identifiers. A release without a
// prerelease suffix is always greater than one with a suffix.
func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] == right[index] {
			continue
		}
		leftNumber, leftIsNumber := numericIdentifier(left[index])
		rightNumber, rightIsNumber := numericIdentifier(right[index])
		switch {
		case leftIsNumber && rightIsNumber:
			return sign(leftNumber - rightNumber)
		case leftIsNumber:
			return -1
		case rightIsNumber:
			return 1
		default:
			return strings.Compare(left[index], right[index])
		}
	}
	return sign(len(left) - len(right))
}

func numericIdentifier(value string) (int, bool) {
	if !isDigits(value) {
		return 0, false
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return number, true
}

func sign(value int) int {
	switch {
	case value < 0:
		return -1
	case value > 0:
		return 1
	default:
		return 0
	}
}

// newerVersion reports whether latest is a release newer than current. When
// current is not a semantic version, such as a local "dev" build, any valid
// release is considered newer.
func newerVersion(latest, current string) bool {
	available, ok := parseVersion(latest)
	if !ok {
		return false
	}
	installed, ok := parseVersion(current)
	if !ok {
		return true
	}
	return available.compare(installed) > 0
}
