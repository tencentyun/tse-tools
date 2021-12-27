package zookeeper

import (
	"hash/fnv"
	"strings"
)

func CompareData(a []byte, b []byte) bool {
	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if (a[i] ^ b[i]) != 0 {
			return false
		}
	}
	return true
}

func HashPath(s string) uint32 {
	alg := fnv.New32a()
	_, _ = alg.Write([]byte(s))
	return alg.Sum32()
}

func ParentOf(path string) string {
	i := strings.LastIndex(path, "/")
	if i == 0 {
		return "/"
	}
	rs := []rune(path)
	return string(rs[0:i])
}

func FullPath(parentPath, childrenName string) string {
	if parentPath == "/" {
		return parentPath + childrenName
	} else {
		return parentPath + "/" + childrenName
	}
}
