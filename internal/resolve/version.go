package resolve

import (
	"strconv"
	"strings"

	"github.com/aaroncutress/kmp-scaffold/internal/catalog"
)

// Version is a parsed Maven-style version string.
type Version struct {
	Raw string
	// Nums are the leading dotted numeric components: 1.5.0-alpha25 -> [1 5 0].
	Nums []int
	// Qualifier is the lowercased text after the numeric prefix, without the
	// leading separator: "alpha25", "rc01", "" for a final release.
	Qualifier string
	// QualRank orders qualifiers: dev < alpha < beta < milestone < rc < final.
	QualRank int
	// QualNum is the trailing number in the qualifier: alpha25 -> 25.
	QualNum int
}

// Qualifier ranks. Final sorts highest so that 1.0 beats 1.0-rc01.
const (
	rankDev = iota
	rankAlpha
	rankBeta
	rankMilestone
	rankRC
	// rankOther covers suffixes that are not prereleases but are variants of a
	// release: "0.8.0-0.6.x-compat", "1.0-jre", "1.0.6-kotlin-2.4.20". They are
	// real releases, so they are not filtered out by channel, but the plain
	// artifact wins when both exist.
	rankOther
	rankFinal
)

// ParseVersion parses a Maven version string. It never fails; unparseable
// input simply sorts low.
func ParseVersion(raw string) Version {
	v := Version{Raw: raw, QualRank: rankFinal}
	s := strings.TrimSpace(raw)
	if s == "" {
		return v
	}

	// Split the leading numeric run from the rest.
	i := 0
	for i < len(s) {
		j := i
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == i {
			break
		}
		n, _ := strconv.Atoi(s[i:j])
		v.Nums = append(v.Nums, n)
		if j < len(s) && s[j] == '.' {
			i = j + 1
			continue
		}
		i = j
		break
	}

	rest := strings.ToLower(strings.TrimLeft(s[i:], "-_."))
	v.Qualifier = rest
	if rest == "" {
		return v
	}

	switch {
	case strings.HasPrefix(rest, "snapshot"), strings.HasPrefix(rest, "dev"),
		strings.HasPrefix(rest, "canary"), strings.HasPrefix(rest, "nightly"):
		v.QualRank = rankDev
	case strings.HasPrefix(rest, "alpha"), strings.HasPrefix(rest, "a") && isDigit(rest, 1):
		v.QualRank = rankAlpha
	case strings.HasPrefix(rest, "beta"), strings.HasPrefix(rest, "b") && isDigit(rest, 1):
		v.QualRank = rankBeta
	case strings.HasPrefix(rest, "milestone"), strings.HasPrefix(rest, "m") && isDigit(rest, 1),
		// Vico and a few others use "-next.N" / "-preview.N" for prereleases.
		strings.HasPrefix(rest, "next"), strings.HasPrefix(rest, "preview"),
		strings.HasPrefix(rest, "pre"), strings.HasPrefix(rest, "eap"):
		v.QualRank = rankMilestone
	case strings.HasPrefix(rest, "rc"), strings.HasPrefix(rest, "cr"):
		v.QualRank = rankRC
	default:
		v.QualRank = rankOther
	}
	v.QualNum = trailingNumber(rest)
	return v
}

func isDigit(s string, i int) bool {
	return i < len(s) && s[i] >= '0' && s[i] <= '9'
}

func trailingNumber(s string) int {
	end := len(s)
	start := end
	for start > 0 && s[start-1] >= '0' && s[start-1] <= '9' {
		start--
	}
	if start == end {
		return 0
	}
	n, _ := strconv.Atoi(s[start:end])
	return n
}

// Channel classifies the version as stable, preview or bleeding edge.
func (v Version) Channel() catalog.Channel {
	switch v.QualRank {
	case rankFinal, rankOther:
		return catalog.Stable
	case rankRC, rankBeta, rankMilestone:
		return catalog.Preview
	default:
		return catalog.Bleeding
	}
}

// Compare returns -1, 0 or 1 comparing v against other.
func (v Version) Compare(other Version) int {
	n := len(v.Nums)
	if len(other.Nums) > n {
		n = len(other.Nums)
	}
	for i := range n {
		a, b := 0, 0
		if i < len(v.Nums) {
			a = v.Nums[i]
		}
		if i < len(other.Nums) {
			b = other.Nums[i]
		}
		if a != b {
			if a < b {
				return -1
			}
			return 1
		}
	}
	if v.QualRank != other.QualRank {
		if v.QualRank < other.QualRank {
			return -1
		}
		return 1
	}
	if v.QualNum != other.QualNum {
		if v.QualNum < other.QualNum {
			return -1
		}
		return 1
	}
	return strings.Compare(v.Raw, other.Raw)
}

// Less reports whether v sorts before other.
func (v Version) Less(other Version) bool { return v.Compare(other) < 0 }

// Major returns the first numeric component, or 0.
func (v Version) Major() int {
	if len(v.Nums) == 0 {
		return 0
	}
	return v.Nums[0]
}

// Minor returns the second numeric component, or 0.
func (v Version) Minor() int {
	if len(v.Nums) < 2 {
		return 0
	}
	return v.Nums[1]
}

// AtLeast reports whether v >= other.
func (v Version) AtLeast(other Version) bool { return v.Compare(other) >= 0 }

// Pick returns the highest version from candidates whose channel is no more
// adventurous than want. If nothing qualifies it returns the highest version
// overall and reports relaxed=true, so callers can explain the fallback.
func Pick(candidates []string, want catalog.Channel) (best string, relaxed bool) {
	var bestV Version
	var anyV Version
	var anyRaw string
	found := false
	for _, c := range candidates {
		v := ParseVersion(c)
		if anyRaw == "" || anyV.Less(v) {
			anyV, anyRaw = v, c
		}
		if v.Channel().Rank() > want.Rank() {
			continue
		}
		if !found || bestV.Less(v) {
			bestV, best, found = v, c, true
		}
	}
	if found {
		return best, false
	}
	return anyRaw, true
}

// EffectiveChannel combines the user's channel with a per-key minimum.
func EffectiveChannel(user, min catalog.Channel) catalog.Channel {
	if min.Rank() > user.Rank() {
		return min
	}
	return user
}

// HighestWithPrefix returns the highest candidate starting with prefix.
func HighestWithPrefix(candidates []string, prefix string, want catalog.Channel) string {
	var filtered []string
	for _, c := range candidates {
		if strings.HasPrefix(c, prefix) {
			filtered = append(filtered, c)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	best, _ := Pick(filtered, want)
	return best
}
