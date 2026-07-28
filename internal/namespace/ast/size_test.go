// Copyright © 2026 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package ast_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ory/keto/internal/namespace/ast"
)

// The expected values are exact struct sizes on 64-bit platforms:
// Relation 48, RelationType 32, SubjectSetRewrite 32, ComputedSubjectSet 16,
// TupleToSubjectSet 32, InvertResult 16, interface header 16, plus string
// lengths. If this test breaks, an AST type changed: update the size
// accounting in size.go together with these numbers.
func TestRelationEstimatedSize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		relation ast.Relation
		expected int64
	}{
		{
			name:     "empty relation",
			relation: ast.Relation{},
			expected: 48,
		},
		{
			name: "relation with name and types",
			relation: ast.Relation{
				Name: "viewers",
				Types: []ast.RelationType{
					{Namespace: "User"},
					{Namespace: "Group", Relation: "members"},
				},
			},
			expected: 48 + 7 + // struct + "viewers"
				32 + 4 + // type + "User"
				32 + 5 + 7, // type + "Group" + "members"
		},
		{
			name: "relation with a rewrite of every node type",
			relation: ast.Relation{
				Name: "view",
				SubjectSetRewrite: &ast.SubjectSetRewrite{
					Operation: ast.OperatorAnd,
					Children: ast.Children{
						&ast.ComputedSubjectSet{Relation: "editors"},
						&ast.TupleToSubjectSet{Relation: "parents", ComputedSubjectSetRelation: "view"},
						&ast.InvertResult{Child: &ast.ComputedSubjectSet{Relation: "banned"}},
						&ast.SubjectSetRewrite{Children: ast.Children{
							&ast.ComputedSubjectSet{Relation: "owners"},
						}},
					},
				},
			},
			expected: 48 + 4 + // struct + "view"
				32 + // rewrite
				16 + 16 + 7 + // header + computed + "editors"
				16 + 32 + 7 + 4 + // header + tuple + "parents" + "view"
				16 + 16 + 16 + 6 + // header + invert + computed + "banned"
				16 + 32 + 16 + 16 + 6, // header + nested rewrite + header + computed + "owners"
		},
		{
			name: "nil children are safe and cost nothing",
			relation: ast.Relation{
				SubjectSetRewrite: &ast.SubjectSetRewrite{
					Children: ast.Children{
						nil,
						(*ast.SubjectSetRewrite)(nil),
						(*ast.ComputedSubjectSet)(nil),
						(*ast.TupleToSubjectSet)(nil),
						(*ast.InvertResult)(nil),
						&ast.InvertResult{},
					},
				},
			},
			expected: 48 + 32 + 6*16 + 16, // struct + rewrite + 6 headers + empty invert
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, tc.relation.EstimatedSize())
		})
	}
}
