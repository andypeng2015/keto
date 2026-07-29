// Copyright © 2023 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package helpers

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	rts "github.com/ory/keto/gen/go/ory/keto/relation_tuples/v1alpha2"
	"github.com/ory/keto/ketoapi"
)

func ParseSubject(s string) (*rts.Subject, error) {
	if strings.Contains(s, ":") {
		su, err := (&ketoapi.SubjectSet{}).FromString(s)
		if err != nil {
			return nil, err
		}

		return rts.NewSubjectSet(su.Namespace, su.Object, su.Relation), nil
	}
	return rts.NewSubjectID(s), nil
}

// ParseNamespaceObject parses namespace and object from args that may be in the
// combined format ["namespace:object"] or the legacy format ["namespace", "object"].
// It writes a deprecation warning to cmd.ErrOrStderr if the legacy format is used.
func ParseNamespaceObject(cmd *cobra.Command, args []string) (namespace, object string, err error) {
	switch len(args) {
	case 2:
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: passing namespace and object as separate arguments is deprecated. Use <object_namespace>:<object_id> instead.")
		return args[0], args[1], nil
	case 1:
		namespace, object, ok := strings.Cut(args[0], ":")
		// empty ObjectID is allowed
		if !ok || namespace == "" {
			return "", "", fmt.Errorf("expected <object_namespace>:<object_id> format, got %q", args[0])
		}
		return namespace, object, nil
	default:
		return "", "", fmt.Errorf("unexpected number of arguments for <object_namespace>:<object_id>: got %d arguments - %s", len(args), strings.Join(args, ","))
	}
}
