/*
Copyright The Platform Mesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package names_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.platform-mesh.io/platform-mesh-deployer/pkg/names"
)

// Limits kcp-operator's derived names are subject to.
const (
	// commonNameMax is the RFC 5280 ub-common-name upper bound cert-manager enforces.
	commonNameMax = 64
	// dnsLabelMax is the DNS-1035 label limit on Service names.
	dnsLabelMax = 63
)

// TestBudgets pins every Max* to the tightest name kcp-operator derives from
// it. The derivations are duplicated from kcp-operator's internal/resources,
// which is not importable; this test is what keeps the copies honest.
func TestBudgets(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		kind    string
		budget  int
		derived []struct {
			format string
			limit  int
		}
	}{
		{
			kind:   "RootShard",
			budget: names.MaxRootShard,
			derived: []struct {
				format string
				limit  int
			}{
				// rootshard/certificates.go: CommonName = GetRootShardCertificateName(r, "service-account")
				{"%s-service-account", commonNameMax},
				// rootshard/certificates.go: CommonName = rootShard.Name
				{"%s", commonNameMax},
				// resources.go: GetRootShardServiceName
				{"%s-kcp", dnsLabelMax},
				// resources.go: GetRootShardProxyServiceName
				{"%s-proxy", dnsLabelMax},
			},
		},
		{
			kind:   "Shard",
			budget: names.MaxShard,
			derived: []struct {
				format string
				limit  int
			}{
				// shard/certificates.go: CommonName = "external-logical-cluster-admin-shard-<name>"
				{"external-logical-cluster-admin-shard-%s", commonNameMax},
				// shard/certificates.go: CommonName = "logical-cluster-admin-shard-<name>"
				{"logical-cluster-admin-shard-%s", commonNameMax},
				// shard/certificates.go: CommonName = "shard-<name>"
				{"shard-%s", commonNameMax},
				// shard/certificates.go: CommonName = GetShardCertificateName(s, certKind)
				{"%s-external-logical-cluster-admin", commonNameMax},
				// resources.go: GetShardServiceName
				{"%s-shard-kcp", dnsLabelMax},
			},
		},
		{
			kind:   "FrontProxy",
			budget: names.MaxFrontProxy,
			derived: []struct {
				format string
				limit  int
			}{
				// resources.go: GetFrontProxyServiceName
				{"%s-front-proxy", dnsLabelMax},
			},
		},
		{
			kind:   "CacheServer",
			budget: names.MaxCacheServer,
			derived: []struct {
				format string
				limit  int
			}{
				// cacheserver/certificates.go: CommonName = GetCacheServerCAName(name, RootCA)
				{"%s-ca", commonNameMax},
				// resources.go: GetCacheServerServiceName
				{"%s-cache-server", dnsLabelMax},
			},
		},
		{
			kind:   "VirtualWorkspace",
			budget: names.MaxVirtualWorkspace,
			derived: []struct {
				format string
				limit  int
			}{
				// virtualworkspace/certificate.go: CommonName = "<name>-virtual-workspace"
				{"%s-virtual-workspace", commonNameMax},
				// resources.go: GetVirtualWorkspaceBaseHost, the Service name
				{"%s-virtual-workspace", dnsLabelMax},
			},
		},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			name := strings.Repeat("x", tc.budget)
			for _, d := range tc.derived {
				derived := strings.Replace(d.format, "%s", name, 1)
				assert.LessOrEqualf(t, len(derived), d.limit,
					"%s budget %d exceeds the %d byte limit on %q", tc.kind, tc.budget, d.limit, d.format)
			}
			// The budget must be the tightest one, not merely a safe one.
			over := strings.Repeat("x", tc.budget+1)
			var tightest bool
			for _, d := range tc.derived {
				if len(strings.Replace(d.format, "%s", over, 1)) > d.limit {
					tightest = true
				}
			}
			assert.Truef(t, tightest, "%s budget %d is lower than necessary", tc.kind, tc.budget)
		})
	}
}

func TestScoped(t *testing.T) {
	t.Parallel()

	t.Run("stays readable within budget", func(t *testing.T) {
		t.Parallel()
		got := names.Scoped(names.MaxShard, "customer-a", "default", "192-168-1-1")
		assert.True(t, strings.HasPrefix(got, "customer-a-default-"), got)
		assert.LessOrEqual(t, len(got), names.MaxShard)
	})

	t.Run("bounded for every budget", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("long", 40)
		for _, max := range []int{
			names.MaxRootShard, names.MaxShard, names.MaxFrontProxy,
			names.MaxCacheServer, names.MaxVirtualWorkspace,
		} {
			assert.LessOrEqual(t, len(names.Scoped(max, long, long, long)), max)
			assert.LessOrEqual(t, len(names.Scoped(max, "a", "b", "c")), max)
		}
	})

	t.Run("cluster id length does not consume budget", func(t *testing.T) {
		t.Parallel()
		short := names.Scoped(names.MaxShard, "customer-a", "default", "east")
		long := names.Scoped(names.MaxShard, "customer-a", "default", strings.Repeat("cluster", 20))
		assert.Equal(t, len(short), len(long))
	})

	t.Run("distinct inputs stay distinct", func(t *testing.T) {
		t.Parallel()
		seen := map[string]string{}
		for _, in := range [][3]string{
			{"customer-a", "default", "east"},
			{"customer-a", "default", "west"},
			{"customer-b", "default", "east"},
			{"customer-a", "other", "east"},
			// Truncated on every budget, differing only past the cut.
			{strings.Repeat("x", 60) + "a", "default", "east"},
			{strings.Repeat("x", 60) + "b", "default", "east"},
		} {
			got := names.Scoped(names.MaxShard, in[0], in[1], in[2])
			prev, dup := seen[got]
			require.Falsef(t, dup, "%v and %s both produced %q", in, prev, got)
			seen[got] = strings.Join(in[:], "/")
		}
	})

	t.Run("is deterministic", func(t *testing.T) {
		t.Parallel()
		a := names.Scoped(names.MaxShard, "customer-a", "default", "east")
		b := names.Scoped(names.MaxShard, "customer-a", "default", "east")
		assert.Equal(t, a, b)
	})

	t.Run("is a valid dns subdomain", func(t *testing.T) {
		t.Parallel()
		for _, got := range []string{
			names.Scoped(names.MaxShard, "customer-a", "default", "east"),
			names.Scoped(names.MaxShard, strings.Repeat("x", 60), "default", "east"),
		} {
			assert.Regexp(t, `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, got)
		}
	})
}
