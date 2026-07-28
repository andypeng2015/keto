// Copyright © 2026 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package ast

import "unsafe"

// EstimatedSize approximates the memory in bytes retained by the relation.
func (r *Relation) EstimatedSize() int64 {
	s := int64(unsafe.Sizeof(*r)) + int64(len(r.Name))
	for _, t := range r.Types {
		s += int64(unsafe.Sizeof(t)) + int64(len(t.Namespace)+len(t.Relation))
	}
	return s + r.SubjectSetRewrite.EstimatedSize()
}

func (r *SubjectSetRewrite) EstimatedSize() int64 {
	if r == nil {
		return 0
	}
	s := int64(unsafe.Sizeof(*r))
	for _, c := range r.Children {
		s += int64(unsafe.Sizeof(c)) + childSize(c)
	}
	return s
}

func (c *ComputedSubjectSet) EstimatedSize() int64 {
	if c == nil {
		return 0
	}
	return int64(unsafe.Sizeof(*c)) + int64(len(c.Relation))
}

func (t *TupleToSubjectSet) EstimatedSize() int64 {
	if t == nil {
		return 0
	}
	return int64(unsafe.Sizeof(*t)) + int64(len(t.Relation)+len(t.ComputedSubjectSetRelation))
}

func (i *InvertResult) EstimatedSize() int64 {
	if i == nil {
		return 0
	}
	return int64(unsafe.Sizeof(*i)) + childSize(i.Child)
}

func childSize(c Child) int64 {
	if c == nil {
		return 0
	}
	return c.EstimatedSize()
}
