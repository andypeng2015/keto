// Copyright © 2023 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/ory/herodot"
	"github.com/ory/x/configx"
	"github.com/ory/x/fetcher"
	"github.com/ory/x/logrusx"
)

func TestNewOPLConfigWatcher(t *testing.T) {
	var hits atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if _, err := io.WriteString(w, testOPL); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(ts.Close)
	ctx := context.Background()
	cfg, err := NewDefault(ctx, nil, logrusx.New("", ""), configx.SkipValidation())
	require.NoError(t, err)
	cw, err := newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
	require.NoError(t, err)
	require.EqualValues(t, 1, hits.Load(), "HTTP request made")
	_, err = cw.GetNamespaceByName(ctx, "User")
	require.NoError(t, err)
	_, err = cw.GetNamespaceByName(ctx, "Document")
	require.NoError(t, err)

	cache.Wait()

	cw, err = newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
	require.NoError(t, err)
	require.EqualValues(t, 1, hits.Load(), "content was cached")
	_, err = cw.GetNamespaceByName(ctx, "User")
	require.NoError(t, err)
	_, err = cw.GetNamespaceByName(ctx, "Document")
	require.NoError(t, err)
}

func TestOPLConfigWatcher(t *testing.T) {
	t.Parallel()

	t.Run("caches parsed namespaces", func(t *testing.T) {
		t.Parallel()
		ts, hits := newCountingServer(t, http.StatusOK, testOPL)
		cfg := newTestConfig(t)
		ctx := t.Context()

		first, err := newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
		require.NoError(t, err)
		cache.Wait()

		second, err := newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
		require.NoError(t, err)
		require.EqualValues(t, 1, hits.Load(), "content was fetched only once")

		firstUser, err := first.GetNamespaceByName(ctx, "User")
		require.NoError(t, err)
		secondUser, err := second.GetNamespaceByName(ctx, "User")
		require.NoError(t, err)
		require.Same(t, firstUser, secondUser, "the parsed namespaces are shared, not re-parsed per construction")
	})

	t.Run("deduplicates concurrent fetches", func(t *testing.T) {
		const parallel = 5
		const target = "https://opl.invalid/namespaces.ts"
		// The cache is process-global and outlives the test binary's runs.
		cache.Del(target)
		// The config warms package-level caches whose goroutines live outside
		// the bubble, so build it here.
		cfg := newTestConfig(t)

		synctest.Test(t, func(t *testing.T) {
			var hits atomic.Int64
			release := make(chan struct{})
			hc := retryablehttp.NewClient()
			hc.Logger = nil
			hc.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				hits.Add(1)
				<-release
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(testOPL)),
					Header:     make(http.Header),
					Request:    r,
				}, nil
			})}
			newFetcher := func() *fetcher.Fetcher { return fetcher.NewFetcher(fetcher.WithClient(hc)) }
			ctx := t.Context()

			watchers := make([]*oplConfigWatcher, parallel)
			var eg errgroup.Group
			for i := range parallel {
				eg.Go(func() error {
					cw, err := newOPLConfigWatcher(ctx, cfg.l, newFetcher, target)
					watchers[i] = cw
					return err
				})
			}
			synctest.Wait() // One caller is inside the fetch, the rest joined its flight.
			close(release)

			require.NoError(t, eg.Wait())
			require.EqualValues(t, 1, hits.Load(), "concurrent constructions share a single fetch")

			for _, cw := range watchers {
				_, err := cw.GetNamespaceByName(ctx, "User")
				require.NoError(t, err)
			}
		})
	})

	t.Run("retries failed fetches", func(t *testing.T) {
		t.Parallel()
		ts, hits := newCountingServer(t, http.StatusNotFound, "")
		cfg := newTestConfig(t)
		ctx := t.Context()

		_, err := newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
		require.ErrorContains(t, err, "404")
		cache.Wait()

		_, err = newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
		require.ErrorContains(t, err, "404")
		require.EqualValues(t, 2, hits.Load(), "failed fetches are not cached")
	})

	t.Run("caches parse failures", func(t *testing.T) {
		t.Parallel()
		ts, hits := newCountingServer(t, http.StatusOK, "/* unclosed comment")
		cfg := newTestConfig(t)
		ctx := t.Context()

		first, err := newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
		require.NoError(t, err, "a parse failure does not fail construction")
		_, err = first.GetNamespaceByName(ctx, "User")
		require.ErrorIs(t, err, herodot.ErrNotFound())
		cache.Wait()

		second, err := newOPLConfigWatcher(ctx, cfg.l, cfg.Fetcher, ts.URL)
		require.NoError(t, err)
		require.EqualValues(t, 1, hits.Load(), "a parse failure is cached and does not trigger a fetch per construction")
		_, err = second.GetNamespaceByName(ctx, "User")
		require.ErrorIs(t, err, herodot.ErrNotFound())
	})
}

func newTestConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := NewDefault(t.Context(), nil, logrusx.New("", ""), configx.SkipValidation())
	require.NoError(t, err)
	return cfg
}

func newCountingServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	hits := new(atomic.Int64)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(ts.Close)
	return ts, hits
}

var testOPL = `
import { Namespace } from "@ory/keto-namespace-types"

class User implements Namespace {}
class Document implements Namespace {}
`

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
