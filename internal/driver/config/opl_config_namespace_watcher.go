// Copyright © 2023 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/ory/x/fetcher"
	"github.com/ory/x/logrusx"
	"github.com/ory/x/urlx"
	"github.com/ory/x/watcherx"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"

	"github.com/ory/keto/internal/namespace"
	"github.com/ory/keto/schema"
)

type (
	configFiles struct {
		byPath map[string]io.Reader
		sync.Mutex
	}

	oplConfigWatcher struct {
		logger *logrusx.Logger
		target string
		files  configFiles

		memoryNamespaceManager
	}
)

var (
	_ namespace.Manager = (*oplConfigWatcher)(nil)

	// cache holds the parsed namespaces per remote OPL location. Content at a location is
	// assumed to be immutable, therefore, a change to the OPL should create a new file location,
	// therefore a new cache entry.
	cache, _ = ristretto.NewCache(&ristretto.Config[string, []*namespace.Namespace]{
		MaxCost:     20_000_000, // 20 MB of estimated heap, each item ca. 10 KB => max 2000 items
		NumCounters: 20_000,     // max 2000 items => 20000 counters
		BufferItems: 64,
		Cost:        namespace.EstimatedSize,
	})

	fetchGroup singleflight.Group
)

func newOPLConfigWatcher(ctx context.Context, l *logrusx.Logger, newFetcher func() *fetcher.Fetcher, target string) (*oplConfigWatcher, error) {
	nw := &oplConfigWatcher{
		logger:                 l,
		target:                 target,
		files:                  configFiles{byPath: make(map[string]io.Reader)},
		memoryNamespaceManager: *NewMemoryNamespaceManager(),
	}

	targetUrl, err := urlx.Parse(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	switch targetUrl.Scheme {
	case "file", "":
		return nw, watchTarget(ctx, target, nw, l)
	case "base64":
		file, err := newFetcher().FetchContext(ctx, target)
		if err != nil {
			return nil, err
		}
		nw.files.Lock()
		defer nw.files.Unlock()
		nw.files.byPath[targetUrl.String()] = file
		nw.parseFiles()
		return nw, err
	case "http", "https":
		if namespaces, ok := cache.Get(target); ok {
			nw.set(namespaces)
			return nw, nil
		}
		v, err, _ := fetchGroup.Do(target, func() (any, error) {
			// Detach from the leader's context so that its cancellation does not
			// fail the concurrent callers sharing this fetch.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()

			buf, err := newFetcher().FetchContext(ctx, target)
			if err != nil {
				return nil, err
			}
			b := buf.Bytes()
			nw.files.Lock()
			defer nw.files.Unlock()
			nw.files.byPath[targetUrl.String()] = bytes.NewReader(b)
			namespaces, ok := nw.parseFiles()
			if !ok {
				// Cache a parse failure as an empty result, so that a broken
				// OPL file does not trigger a fetch per construction.
				namespaces = []*namespace.Namespace{}
			}
			// Cost 0 lets the cache's Cost function compute the entry size.
			cache.SetWithTTL(target, namespaces, 0, 30*time.Minute)
			return namespaces, nil
		})
		if err != nil {
			return nil, err
		}
		if namespaces, ok := v.([]*namespace.Namespace); ok {
			nw.set(namespaces)
		}
		return nw, nil
	default:
		return nil, fmt.Errorf("unexpected url scheme: %q", targetUrl.Scheme)
	}
}

func (nw *oplConfigWatcher) handleChange(e *watcherx.ChangeEvent) {
	// the lock is acquired before parsing to ensure that the getters are
	// waiting for the updated values
	nw.files.Lock()
	defer nw.files.Unlock()
	nw.files.byPath[e.Source()] = e.Reader()
	nw.parseFiles()
}

func (nw *oplConfigWatcher) handleRemove(e *watcherx.RemoveEvent) {
	nw.files.Lock()
	defer nw.files.Unlock()
	delete(nw.files.byPath, e.Source())
	nw.parseFiles()
}

func (nw *oplConfigWatcher) handleError(e *watcherx.ErrorEvent) {
	nw.logger.
		WithError(e).
		Errorf("Received error while watching OPL config files at target %s.",
			nw.target)
}

// parseFiles loops through all files, parsing each and getting the namespaces.
// It then sets and returns the namespaces only if there were no errors.
//
// The caller must  hold the lock to nw.files.
func (nw *oplConfigWatcher) parseFiles() ([]*namespace.Namespace, bool) {
	var (
		namespaces = make([]*namespace.Namespace, 0)
		errs       []error
	)
	for _, reader := range nw.files.byPath {
		content, err := io.ReadAll(reader)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		nn, ee := schema.Parse(string(content))
		for _, e := range ee {
			errs = append(errs, e)
		}
		for _, n := range nn {
			namespaces = append(namespaces, &n)
		}
	}
	if len(errs) > 0 {
		for _, err := range errs {
			nw.logger.
				WithError(err).
				Errorf("Failed to parse OPL config files at target %s.",
					nw.target)
		}
		return nil, false
	}
	nw.set(namespaces)
	return namespaces, true
}
