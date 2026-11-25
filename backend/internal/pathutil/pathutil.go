// Package pathutil handles the folder paths secrets live under ("/", "/stripe", "/a/b").
package pathutil

import "strings"

// Norm canonicalises a path ("a//b/" -> "/a/b") and rejects "." and ".." segments and
// segments longer than 64 characters.
func Norm(p string) (string, bool) {
	var parts []string
	for _, seg := range strings.Split(strings.TrimSpace(p), "/") {
		if seg == "" {
			continue
		}
		if seg == "." || seg == ".." || len(seg) > 64 {
			return "", false
		}
		parts = append(parts, seg)
	}
	return "/" + strings.Join(parts, "/"), true
}

// Join appends a key to a folder path.
func Join(path, key string) string {
	if path == "/" {
		return "/" + key
	}
	return path + "/" + key
}

// Within reports whether path is prefix itself or sits below it ("/stripe2" is not within "/stripe").
func Within(prefix, path string) bool {
	return prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/")
}
