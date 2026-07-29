// Copyright © 2023 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package clienttest

import (
	"github.com/ory/x/randx"

	"github.com/ory/keto/ketoapi"
)

func RandomTupleWithSubjectSet(ns1, ns2 string) *ketoapi.RelationTuple {
	return &ketoapi.RelationTuple{
		Namespace: ns1,
		Object:    randx.MustString(8, randx.AlphaNum),
		Relation:  randx.MustString(8, randx.AlphaNum),
		SubjectSet: &ketoapi.SubjectSet{
			Namespace: ns2,
			Object:    randx.MustString(8, randx.AlphaNum),
		},
	}
}

func RandomTupleWithSubjectID(ns1 string) *ketoapi.RelationTuple {
	return &ketoapi.RelationTuple{
		Namespace: ns1,
		Object:    randx.MustString(8, randx.AlphaNum),
		Relation:  randx.MustString(8, randx.AlphaNum),
		SubjectID: new(randx.MustString(8, randx.AlphaNum)),
	}
}
