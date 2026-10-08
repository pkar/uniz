// Package pathutil implements Unix path-string operations without accessing disk.
package pathutil

import "strings"

// BaseName removes directory components, trailing slashes, and an optional
// suffix. A suffix equal to the whole basename is not removed. All-slash paths
// yield "/"; an empty path yields "". Dot components are not cleaned.
func BaseName(path, suffix string) string {
	name := strings.TrimRight(path, "/")
	if name == "" && path != "" {
		name = "/"
	}
	if name != "/" {
		name = name[strings.LastIndex(name, "/")+1:]
	}
	if suffix != "" && suffix != name {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// DirName removes trailing slashes and the final component. It returns "."
// when there is no directory component and "/" for root. Dot components are
// not cleaned. Exactly two leading slashes have no special meaning.
func DirName(path string) string {
	name := strings.TrimRight(path, "/")
	if name == "" {
		if path != "" {
			return "/"
		}
		return "."
	}
	index := strings.LastIndex(name, "/")
	if index < 0 {
		return "."
	}
	parent := strings.TrimRight(name[:index], "/")
	if parent == "" {
		return "/"
	}
	return parent
}
