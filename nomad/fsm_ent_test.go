//go:build ent
// +build ent

package nomad

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/ci"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
	"github.com/shoenig/test/must"
)

func TestFSM_UpsertSentinelPolicies(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)

	policy := mock.SentinelPolicy()
	req := structs.SentinelPolicyUpsertRequest{
		Policies: []*structs.SentinelPolicy{policy},
	}
	buf, err := structs.Encode(structs.SentinelPolicyUpsertRequestType, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	resp := fsm.Apply(makeLog(buf))
	if resp != nil {
		t.Fatalf("resp: %v", resp)
	}

	// Verify we are registered
	ws := memdb.NewWatchSet()
	out, err := fsm.State().SentinelPolicyByName(ws, policy.Name)
	must.Nil(t, err)
	must.NotNil(t, out)
}

func TestFSM_DeleteSentinelPolicies(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)

	policy := mock.SentinelPolicy()
	err := fsm.State().UpsertSentinelPolicies(1000, []*structs.SentinelPolicy{policy})
	must.Nil(t, err)

	req := structs.SentinelPolicyDeleteRequest{
		Names: []string{policy.Name},
	}
	buf, err := structs.Encode(structs.SentinelPolicyDeleteRequestType, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	resp := fsm.Apply(makeLog(buf))
	if resp != nil {
		t.Fatalf("resp: %v", resp)
	}

	// Verify we are NOT registered
	ws := memdb.NewWatchSet()
	out, err := fsm.State().SentinelPolicyByName(ws, policy.Name)
	must.Nil(t, err)
	must.Nil(t, out)
}

func TestFSM_SnapshotRestore_SentinelPolicy(t *testing.T) {
	ci.Parallel(t)
	// Add some state
	fsm := testFSM(t)
	state := fsm.State()
	p1 := mock.SentinelPolicy()
	p2 := mock.SentinelPolicy()
	state.UpsertSentinelPolicies(1000, []*structs.SentinelPolicy{p1, p2})

	// Verify the contents
	fsm2 := testSnapshotRestore(t, fsm)
	state2 := fsm2.State()
	ws := memdb.NewWatchSet()
	out1, _ := state2.SentinelPolicyByName(ws, p1.Name)
	out2, _ := state2.SentinelPolicyByName(ws, p2.Name)
	must.Eq(t, p1, out1)
	must.Eq(t, p2, out2)
}

func TestFSM_UpsertQuotaSpecs(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)

	spec := mock.QuotaSpec()
	req := structs.QuotaSpecUpsertRequest{
		Quotas: []*structs.QuotaSpec{spec},
	}
	buf, err := structs.Encode(structs.QuotaSpecUpsertRequestType, req)
	must.Nil(t, err)

	resp := fsm.Apply(makeLog(buf))
	must.Nil(t, resp)

	// Verify we are registered
	ws := memdb.NewWatchSet()
	out, err := fsm.State().QuotaSpecByName(ws, spec.Name)
	must.Nil(t, err)
	must.NotNil(t, out)

	usage, err := fsm.State().QuotaUsageByName(ws, spec.Name)
	must.Nil(t, err)
	must.NotNil(t, usage)
}

// This test checks that unblocks are triggered when a quota changes
func TestFSM_UpsertQuotaSpecs_Modify(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)
	state := fsm.State()
	fsm.blockedEvals.SetEnabled(true)

	// Create a quota specs
	qs1 := mock.QuotaSpec()
	must.Nil(t, state.UpsertQuotaSpecs(1, []*structs.QuotaSpec{qs1}))

	// Block an eval for that namespace
	e := mock.Eval()
	e.QuotaLimitReached = qs1.Name
	fsm.blockedEvals.Block(e)

	bstats := fsm.blockedEvals.Stats()
	must.Eq(t, 1, bstats.TotalBlocked)
	must.Eq(t, 1, bstats.TotalQuotaLimit)

	// Update the namespace to use the new spec
	qs2 := qs1.Copy()
	req := structs.QuotaSpecUpsertRequest{
		Quotas: []*structs.QuotaSpec{qs2},
	}
	buf, err := structs.Encode(structs.QuotaSpecUpsertRequestType, req)
	must.Nil(t, err)
	must.Nil(t, fsm.Apply(makeLog(buf)))

	// Verify we unblocked
	testutil.WaitForResult(func() (bool, error) {
		bStats := fsm.blockedEvals.Stats()
		if bStats.TotalBlocked != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		if bStats.TotalQuotaLimit != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		return true, nil
	}, func(err error) {
		t.Fatalf("err: %s", err)
	})
}

func TestFSM_DeleteQuotaSpecs(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)

	spec := mock.QuotaSpec()
	must.Nil(t, fsm.State().UpsertQuotaSpecs(1000, []*structs.QuotaSpec{spec}))

	req := structs.QuotaSpecDeleteRequest{
		Names: []string{spec.Name},
	}
	buf, err := structs.Encode(structs.QuotaSpecDeleteRequestType, req)
	must.Nil(t, err)

	resp := fsm.Apply(makeLog(buf))
	must.Nil(t, resp)

	// Verify we are NOT registered
	ws := memdb.NewWatchSet()
	out, err := fsm.State().QuotaSpecByName(ws, spec.Name)
	must.Nil(t, err)
	must.Nil(t, out)

	usage, err := fsm.State().QuotaUsageByName(ws, spec.Name)
	must.Nil(t, err)
	must.Nil(t, usage)
}

func TestFSM_SnapshotRestore_QuotaSpec(t *testing.T) {
	ci.Parallel(t)

	// Add some state
	fsm := testFSM(t)
	state := fsm.State()
	qs1 := mock.QuotaSpec()
	qs2 := mock.QuotaSpec()
	must.Nil(t, state.UpsertQuotaSpecs(1000, []*structs.QuotaSpec{qs1, qs2}))

	// Verify the contents
	fsm2 := testSnapshotRestore(t, fsm)
	state2 := fsm2.State()
	ws := memdb.NewWatchSet()
	out1, _ := state2.QuotaSpecByName(ws, qs1.Name)
	out2, _ := state2.QuotaSpecByName(ws, qs2.Name)
	must.Eq(t, qs1, out1)
	must.Eq(t, qs2, out2)
}

func TestFSM_SnapshotRestore_QuotaUsage(t *testing.T) {
	ci.Parallel(t)

	// Add some state
	fsm := testFSM(t)
	state := fsm.State()
	qs1 := mock.QuotaSpec()
	qs2 := mock.QuotaSpec()
	must.Nil(t, state.UpsertQuotaSpecs(999, []*structs.QuotaSpec{qs1, qs2}))
	qu1 := mock.QuotaUsage()
	qu2 := mock.QuotaUsage()
	qu1.Name = qs1.Name
	qu2.Name = qs2.Name

	// QuotaUsage is reconciled on insertion, so usage should come back 0
	must.Nil(t, state.UpsertQuotaUsages(1000, []*structs.QuotaUsage{qu1, qu2}))
	qu1Out, _ := state.QuotaUsageByName(nil, qu1.Name)
	must.NotEq(t, qu1, qu1Out)
	for _, used := range qu1Out.Used {
		must.Zero(t, used.RegionLimit.CPU)
		must.Zero(t, used.RegionLimit.MemoryMB)
		must.Zero(t, used.RegionLimit.MemoryMaxMB)
	}

	qu2Out, _ := state.QuotaUsageByName(nil, qu2.Name)
	must.NotEq(t, qu2, qu2Out)
	for _, used := range qu2Out.Used {
		must.Zero(t, used.RegionLimit.CPU)
		must.Zero(t, used.RegionLimit.MemoryMB)
		must.Zero(t, used.RegionLimit.MemoryMaxMB)
	}

	// Verify the contents
	fsm2 := testSnapshotRestore(t, fsm)
	state2 := fsm2.State()
	out1, _ := state2.QuotaUsageByName(nil, qu1.Name)
	out2, _ := state2.QuotaUsageByName(nil, qu2.Name)
	must.Eq(t, qu1Out, out1)
	must.Eq(t, qu2Out, out2)
}

// This test checks that unblocks are triggered when an alloc is updated and it
// has an associated quota.
func TestFSM_AllocClientUpdate_Quota(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)
	state := fsm.State()
	fsm.blockedEvals.SetEnabled(true)

	// Create a quota specs
	qs1 := mock.QuotaSpec()
	must.Nil(t, state.UpsertQuotaSpecs(1, []*structs.QuotaSpec{qs1}))

	// Create a namespace
	ns := mock.Namespace()
	must.Nil(t, state.UpsertNamespaces(2, []*structs.Namespace{ns}))

	// Create the node
	node := mock.Node()
	state.UpsertNode(structs.MsgTypeTestSetup, 3, node)

	// Block an eval for that namespace
	e := mock.Eval()
	e.Namespace = ns.Name
	e.QuotaLimitReached = qs1.Name
	fsm.blockedEvals.Block(e)

	bstats := fsm.blockedEvals.Stats()
	must.Eq(t, 1, bstats.TotalBlocked)
	must.Eq(t, 1, bstats.TotalQuotaLimit)

	// Create an alloc to update
	alloc := mock.Alloc()
	alloc.Namespace = ns.Name
	alloc.NodeID = node.ID
	alloc2 := mock.Alloc()
	alloc2.Namespace = ns.Name
	alloc2.NodeID = node.ID
	state.UpsertAllocs(structs.MsgTypeTestSetup, 10, []*structs.Allocation{alloc, alloc2})

	clientAlloc := alloc.Copy()
	clientAlloc.ClientStatus = structs.AllocClientStatusComplete
	update2 := &structs.Allocation{
		ID:           alloc2.ID,
		NodeID:       node.ID,
		Namespace:    ns.Name,
		ClientStatus: structs.AllocClientStatusRunning,
	}

	req := structs.AllocUpdateRequest{
		Alloc: []*structs.Allocation{clientAlloc, update2},
	}
	buf, err := structs.Encode(structs.AllocClientUpdateRequestType, req)
	must.Nil(t, err)

	resp := fsm.Apply(makeLog(buf))
	must.Nil(t, resp)

	// Verify we unblocked
	testutil.WaitForResult(func() (bool, error) {
		bStats := fsm.blockedEvals.Stats()
		if bStats.TotalBlocked != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		if bStats.TotalQuotaLimit != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		return true, nil
	}, func(err error) {
		t.Fatalf("err: %s", err)
	})
}

// This test checks that unblocks are triggered when a namespace changes its
// quota
func TestFSM_UpsertNamespaces_ModifyQuota(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)
	state := fsm.State()
	fsm.blockedEvals.SetEnabled(true)

	// Create two quota specs
	qs1 := mock.QuotaSpec()
	qs2 := mock.QuotaSpec()
	must.Nil(t, state.UpsertQuotaSpecs(1, []*structs.QuotaSpec{qs1, qs2}))

	// Create a namepace
	ns1 := mock.Namespace()
	ns1.Quota = qs1.Name
	must.Nil(t, state.UpsertNamespaces(2, []*structs.Namespace{ns1}))

	// Block an eval for that namespace
	e := mock.Eval()
	e.QuotaLimitReached = qs1.Name
	fsm.blockedEvals.Block(e)

	bstats := fsm.blockedEvals.Stats()
	must.Eq(t, 1, bstats.TotalBlocked)
	must.Eq(t, 1, bstats.TotalQuotaLimit)

	// Update the namespace to use the new spec
	ns2 := ns1.Copy()
	ns2.Quota = qs2.Name
	req := structs.NamespaceUpsertRequest{
		Namespaces: []*structs.Namespace{ns2},
	}
	buf, err := structs.Encode(structs.NamespaceUpsertRequestType, req)
	must.Nil(t, err)
	must.Nil(t, fsm.Apply(makeLog(buf)))

	// Verify we unblocked
	testutil.WaitForResult(func() (bool, error) {
		bStats := fsm.blockedEvals.Stats()
		if bStats.TotalBlocked != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		if bStats.TotalQuotaLimit != 0 {
			return false, fmt.Errorf("bad: %#v", bStats)
		}
		return true, nil
	}, func(err error) {
		t.Fatalf("err: %s", err)
	})
}

func TestFSM_UpsertLicense(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)

	stored, _ := mock.StoredLicense()
	req := structs.LicenseUpsertRequest{
		License: stored,
	}
	buf, err := structs.Encode(structs.LicenseUpsertRequestType, req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	resp := fsm.Apply(makeLog(buf))
	if resp != nil {
		t.Fatalf("resp: %v", resp)
	}

	// Verify we are registered
	ws := memdb.NewWatchSet()
	out, err := fsm.State().License(ws)
	must.Nil(t, err)
	must.NotNil(t, out)
}

func TestFSM_SnapshotRestore_License(t *testing.T) {
	ci.Parallel(t)

	// Add some state
	fsm := testFSM(t)
	state := fsm.State()
	stored, _ := mock.StoredLicense()
	must.Nil(t, state.UpsertLicense(1000, stored))

	// Verify the contents
	fsm2 := testSnapshotRestore(t, fsm)
	state2 := fsm2.State()
	ws := memdb.NewWatchSet()
	out1, _ := state2.License(ws)
	must.NotNil(t, out1)
	must.Eq(t, stored, out1)
}

func TestFSM_SnapshotRestore_TmpLicenseBarrier(t *testing.T) {
	ci.Parallel(t)

	// Add some state
	fsm := testFSM(t)
	state := fsm.State()
	stored := &structs.TmpLicenseBarrier{CreateTime: time.Now().UnixNano()}
	must.Nil(t, state.TmpLicenseSetBarrier(1000, stored))

	// Verify the contents
	fsm2 := testSnapshotRestore(t, fsm)
	state2 := fsm2.State()
	out1, _ := state2.TmpLicenseBarrier(nil)
	must.NotNil(t, out1)
	must.Eq(t, stored, out1)
}

func TestFSM_UpsertRecommdation(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)

	ns := mock.Namespace()
	job := mock.Job()
	job.Namespace = ns.Name
	rec := mock.Recommendation(job)
	must.NoError(t, fsm.State().UpsertNamespaces(1000, []*structs.Namespace{ns}))
	must.NoError(t, fsm.State().UpsertJob(structs.MsgTypeTestSetup, 1010, nil, job))
	req := structs.RecommendationUpsertRequest{
		Recommendation: rec,
	}
	buf, err := structs.Encode(structs.RecommendationUpsertRequestType, req)
	must.NoError(t, err)
	must.Nil(t, fsm.Apply(makeLog(buf)))

	out, err := fsm.State().RecommendationByID(nil, rec.ID)
	must.NoError(t, err)
	must.NotNil(t, out)
}

func TestFSM_DeleteRecommendations(t *testing.T) {
	ci.Parallel(t)
	fsm := testFSM(t)

	ns1 := mock.Namespace()
	ns2 := mock.Namespace()
	must.NoError(t, fsm.State().UpsertNamespaces(1000, []*structs.Namespace{ns1, ns2}))
	job1 := mock.Job()
	job1.Namespace = ns1.Name
	job2 := mock.Job()
	job2.Namespace = ns2.Name
	must.NoError(t, fsm.State().UpsertJob(structs.MsgTypeTestSetup, 1001, nil, job1))
	must.NoError(t, fsm.State().UpsertJob(structs.MsgTypeTestSetup, 1002, nil, job2))
	rec1 := mock.Recommendation(job1)
	rec2 := mock.Recommendation(job2)
	must.NoError(t, fsm.State().UpsertRecommendation(1002, rec1))
	must.NoError(t, fsm.State().UpsertRecommendation(1003, rec2))

	req := structs.RecommendationDeleteRequest{
		Recommendations: []string{rec1.ID, rec2.ID},
	}
	buf, err := structs.Encode(structs.RecommendationDeleteRequestType, req)
	must.NoError(t, err)
	must.Nil(t, fsm.Apply(makeLog(buf)))

	out, err := fsm.State().RecommendationByID(nil, rec1.ID)
	must.NoError(t, err)
	must.Nil(t, out)

	out, err = fsm.State().RecommendationByID(nil, rec2.ID)
	must.NoError(t, err)
	must.Nil(t, out)
}

func TestFSM_SnapshotRestore_Recommendations(t *testing.T) {
	ci.Parallel(t)
	// Add some state
	fsm := testFSM(t)
	state := fsm.State()
	job1 := mock.Job()
	job2 := mock.Job()
	rec1 := mock.Recommendation(job1)
	rec2 := mock.Recommendation(job2)
	state.UpsertJob(structs.MsgTypeTestSetup, 1000, nil, job1)
	state.UpsertJob(structs.MsgTypeTestSetup, 1001, nil, job2)
	state.UpsertRecommendation(1002, rec1)
	state.UpsertRecommendation(1003, rec2)

	// Verify the contents
	fsm2 := testSnapshotRestore(t, fsm)
	state2 := fsm2.State()
	ws := memdb.NewWatchSet()
	out1, _ := state2.RecommendationsByJob(ws, job1.Namespace, job1.ID, nil)
	must.Len(t, 1, out1)
	must.Eq(t, rec1.Value, out1[0].Value)
	out1[0].Value = rec1.Value
	must.True(t, reflect.DeepEqual(rec1, out1[0]))
	out2, _ := state2.RecommendationsByJob(ws, job2.Namespace, job2.ID, nil)
	must.Len(t, 1, out2)
	must.Eq(t, rec2.Value, out2[0].Value)
	out2[0].Value = rec2.Value
	must.True(t, reflect.DeepEqual(rec2, out2[0]))
}