// Package dedupe implements the URL canonicalization and ID derivation
// rules from docs/design.md §5 ("Deduplication").
package dedupe

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
)

// trackingParamPrefixes are stripped from query strings before hashing.
var trackingParamPrefixes = []string{"utm_", "gh_src", "lever-source", "ref", "fbclid", "gclid"}

// CanonicalizeURL lowercases the host, strips tracking query params and the
// fragment, and returns a stable string suitable for hashing into a
// PostingID.
func CanonicalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""

	q := u.Query()
	for key := range q {
		lower := strings.ToLower(key)
		for _, prefix := range trackingParamPrefixes {
			if strings.HasPrefix(lower, prefix) {
				q.Del(key)
				break
			}
		}
	}
	// Sort remaining params so equivalent URLs hash identically regardless
	// of original ordering.
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sorted := url.Values{}
	for _, k := range keys {
		sorted[k] = q[k]
	}
	u.RawQuery = sorted.Encode()

	return u.String(), nil
}

// PostingID derives the stable posting ID: sha256(canonicalURL)[:16] hex
// chars, per docs/design.md §4.
func PostingID(canonicalURL string) string {
	sum := sha256.Sum256([]byte(canonicalURL))
	return hex.EncodeToString(sum[:])[:16]
}

// ContentHash hashes normalized body text so re-crawls can detect edits and
// trigger re-extraction (docs/design.md §5).
func ContentHash(normalizedText string) string {
	sum := sha256.Sum256([]byte(normalizedText))
	return hex.EncodeToString(sum[:])
}
