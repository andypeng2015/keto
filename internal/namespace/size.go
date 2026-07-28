// Copyright © 2026 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package namespace

import "unsafe"

// EstimatedSize approximates the memory in bytes retained by the namespaces.
func EstimatedSize(nn []*Namespace) int64 {
	var s int64
	for _, n := range nn {
		if n == nil {
			continue
		}
		s += int64(unsafe.Sizeof(n)) + int64(unsafe.Sizeof(*n)) + int64(len(n.Name)+len(n.Config))
		for i := range n.Relations {
			s += n.Relations[i].EstimatedSize()
		}
	}
	return s
}
