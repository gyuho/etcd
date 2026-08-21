// Copyright 2026 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.etcd.io/etcd/tests/v3/framework/config"
	"go.etcd.io/etcd/tests/v3/framework/testutils"
)

// TestResponseHeaderLeaderId verifies that every v3 ResponseHeader carries
// the serving member's current raft leader view in header.leader_id, and
// that the value agrees with the leader reported by Status on every member.
func TestResponseHeaderLeaderId(t *testing.T) {
	testRunner.BeforeTest(t)

	for _, tc := range clusterTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			clus := testRunner.NewCluster(ctx, t, config.WithClusterConfig(tc.config))
			defer clus.Close()
			clus.WaitLeader(t)
			cc := testutils.MustClient(clus.Client())

			testutils.ExecuteUntil(ctx, t, func() {
				rs, err := cc.Status(ctx)
				require.NoErrorf(t, err, "could not get status")
				require.Lenf(t, rs, tc.config.ClusterSize, "wrong number of status responses. expected:%d, got:%d", tc.config.ClusterSize, len(rs))

				leader := rs[0].Header.LeaderId
				require.NotZerof(t, leader, "header.leader_id should be set once the cluster has a leader")

				for _, r := range rs {
					require.Equalf(t, r.Leader, r.Header.LeaderId,
						"header.leader_id should match status.leader for member %016x", r.Header.MemberId)
					require.Equalf(t, leader, r.Header.LeaderId,
						"all members should agree on header.leader_id, member %016x reported %016x", r.Header.MemberId, r.Header.LeaderId)
				}

				putResp, err := cc.Put(ctx, "leader-id-key", "v", config.PutOptions{})
				require.NoErrorf(t, err, "could not put key")
				require.Equalf(t, leader, putResp.Header.LeaderId, "put response should carry the same leader_id")

				getResp, err := cc.Get(ctx, "leader-id-key", config.GetOptions{})
				require.NoErrorf(t, err, "could not get key")
				require.Equalf(t, leader, getResp.Header.LeaderId, "get response should carry the same leader_id")
			})
		})
	}
}
