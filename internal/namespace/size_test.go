// Copyright © 2026 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package namespace_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ory/keto/internal/namespace"
	"github.com/ory/keto/internal/namespace/ast"
)

func TestEstimatedSize(t *testing.T) {
	t.Parallel()

	require.Zero(t, namespace.EstimatedSize(nil))
	require.Zero(t, namespace.EstimatedSize([]*namespace.Namespace{}))

	nn := []*namespace.Namespace{
		{Name: "Document", Config: json.RawMessage(`{}`)},
		nil, // Must not panic.
		{Name: "User", Relations: []ast.Relation{{Name: "manager"}}},
	}
	require.EqualValues(t,
		8+72+8+2+ // pointer + Namespace + "Document" + config
			8+72+4+ // pointer + Namespace + "User"
			48+7, // relation + "manager"
		namespace.EstimatedSize(nn))
}
