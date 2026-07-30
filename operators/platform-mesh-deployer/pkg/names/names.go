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

// Package names builds the kcp-operator admin CR names the deployer creates.
//
// kcp-operator derives Deployment, Service, Certificate and X.509 CommonName
// values from the admin CR name, so every name has a budget far below the 253
// byte limit on object names. The budgets are the Max* constants below.
package names

import (
	"crypto/sha256"
	"encoding/hex"
)

// Name budgets, each set by the tightest value kcp-operator derives from the
// admin CR name. Pinned by TestBudgets.
const (
	// MaxRootShard is bounded by the "<name>-service-account" CommonName.
	MaxRootShard = 48
	// MaxShard is bounded by the "external-logical-cluster-admin-shard-<name>"
	// CommonName, the tightest budget of them all.
	MaxShard = 27
	// MaxFrontProxy is bounded by the "<name>-front-proxy" Service.
	MaxFrontProxy = 51
	// MaxCacheServer is bounded by the "<name>-cache-server" Service.
	MaxCacheServer = 50
	// MaxVirtualWorkspace is bounded by the "<name>-virtual-workspace" Service.
	MaxVirtualWorkspace = 45
)

const (
	// clusterHashLen keeps the cluster segment fixed width so a name's budget
	// does not depend on a provider-controlled cluster ID.
	clusterHashLen = 6
	// overflowHashLen restores uniqueness to a name that had to be truncated.
	overflowHashLen = 8
)

// Scoped returns the admin CR name for a component of platformMesh on the
// given cluster, bounded to max bytes.
//
// The cluster ID is hashed because it is provider-controlled and unbounded,
// while platformMesh and component stay readable. Names over max are truncated
// and suffixed with a hash of the full name so distinct inputs stay distinct.
// The LabelCluster label on every admin CR maps a name back to its cluster.
func Scoped(max int, platformMesh, component, clusterID string) string {
	name := platformMesh + "-" + component + "-" + hash(clusterID, clusterHashLen)
	if len(name) <= max {
		return name
	}
	return name[:max-overflowHashLen-1] + "-" + hash(name, overflowHashLen)
}

func hash(s string, n int) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:n]
}
