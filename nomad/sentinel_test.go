// +build ent

package nomad

import (
	"testing"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
	"github.com/hashicorp/sentinel/sentinel"
	"github.com/stretchr/testify/assert"
)

func TestServer_Sentinel_EnforceScope(t *testing.T) {
	t.Parallel()
	s1, _, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	testutil.WaitForLeader(t, s1.RPC)

	// data call back for enforement
	dataCB := func() map[string]interface{} {
		return map[string]interface{}{
			"foo_bar": false,
		}
	}

	// Create a fake policy
	policy1 := mock.SentinelPolicy()
	policy2 := mock.SentinelPolicy()
	s1.State().UpsertSentinelPolicies(1000,
		[]*structs.SentinelPolicy{policy1, policy2})

	// Check that everything passes
	warn, err := s1.enforceScope(false, structs.SentinelScopeSubmitJob, dataCB)
	assert.Nil(t, err)
	assert.Nil(t, warn)

	// Add a failing policy
	policy3 := mock.SentinelPolicy()
	policy3.EnforcementLevel = structs.SentinelEnforcementLevelHardMandatory
	policy3.Policy = "main = rule { foo_bar }"
	s1.State().UpsertSentinelPolicies(1001, []*structs.SentinelPolicy{policy3})

	// Check that we get an error
	warn, err = s1.enforceScope(false, structs.SentinelScopeSubmitJob, dataCB)
	assert.NotNil(t, err)
	assert.Nil(t, warn)

	// Update policy3 to be advisory
	p3update := new(structs.SentinelPolicy)
	*p3update = *policy3
	p3update.EnforcementLevel = structs.SentinelEnforcementLevelAdvisory
	s1.State().UpsertSentinelPolicies(1002, []*structs.SentinelPolicy{p3update})

	// Check that we get a warning
	warn, err = s1.enforceScope(false, structs.SentinelScopeSubmitJob, dataCB)
	assert.Nil(t, err)
	assert.NotNil(t, warn)
}

func TestServer_Sentinel_EnforceScope_MultiJobPolicies(t *testing.T) {
	t.Parallel()
	s1, _, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	testutil.WaitForLeader(t, s1.RPC)

	// data call back for enforement
	job := mock.Job()
	dataCB := func() map[string]interface{} {
		return map[string]interface{}{
			"job": job,
		}
	}

	// Create a fake policy
	p1 := mock.SentinelPolicy()
	p1.Policy = "main = rule { job.priority == 50 }"
	p2 := mock.SentinelPolicy()
	p2.Policy = "main = rule { job.region == \"global\" }"
	s1.State().UpsertSentinelPolicies(1000,
		[]*structs.SentinelPolicy{p1, p2})

	// Check that everything passes
	warn, err := s1.enforceScope(false, structs.SentinelScopeSubmitJob, dataCB)
	if !assert.Nil(t, err) {
		t.Logf("error: %v", err)
	}
	if !assert.Nil(t, warn) {
		t.Logf("warn: %v", warn)
	}
}

// Ensure the standard lib is present
func TestServer_Sentinel_EnforceScope_Stdlib(t *testing.T) {
	t.Parallel()
	s1, _, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	testutil.WaitForLeader(t, s1.RPC)

	// data call back for enforement
	dataCB := func() map[string]interface{} {
		return map[string]interface{}{
			"my_string": "foobar",
		}
	}

	// Add a policy
	policy1 := mock.SentinelPolicy()
	policy1.EnforcementLevel = structs.SentinelEnforcementLevelHardMandatory
	policy1.Policy = `import "strings"
	main = rule { strings.has_prefix(my_string, "foo") }
	`
	s1.State().UpsertSentinelPolicies(1001, []*structs.SentinelPolicy{policy1})

	// Check that we pass
	warn, err := s1.enforceScope(false, structs.SentinelScopeSubmitJob, dataCB)
	assert.Nil(t, err)
	assert.Nil(t, warn)
}

// Ensure HTTP import is not present
func TestServer_Sentinel_EnforceScope_HTTP(t *testing.T) {
	t.Parallel()
	s1, _ := testACLServer(t, nil)
	defer s1.Shutdown()
	testutil.WaitForLeader(t, s1.RPC)

	// data call back for enforement
	dataCB := func() map[string]interface{} {
		return map[string]interface{}{
			"my_string": "foobar",
		}
	}

	// Add a policy
	policy1 := mock.SentinelPolicy()
	policy1.EnforcementLevel = structs.SentinelEnforcementLevelHardMandatory
	policy1.Policy = `import "http"
	main = rule { true }
	`
	s1.State().UpsertSentinelPolicies(1001, []*structs.SentinelPolicy{policy1})

	// Check that we get an error
	warn, err = s1.enforceScope(false, structs.SentinelScopeSubmitJob, dataCB)
	assert.NotNil(t, err)
	assert.Nil(t, warn)
}

func TestServer_SentinelPoliciesByScope(t *testing.T) {
	t.Parallel()
	s1, _, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	testutil.WaitForLeader(t, s1.RPC)

	// Create a fake policy
	policy1 := mock.SentinelPolicy()
	policy2 := mock.SentinelPolicy()
	s1.State().UpsertSentinelPolicies(1000,
		[]*structs.SentinelPolicy{policy1, policy2})

	// Ensure we get them back
	ps, err := s1.sentinelPoliciesByScope(structs.SentinelScopeSubmitJob)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(ps))
}

func TestServer_PrepareSentinelPolicies(t *testing.T) {
	t.Parallel()
	s1, _, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	testutil.WaitForLeader(t, s1.RPC)

	// Create a fake policy
	policy1 := mock.SentinelPolicy()
	policy2 := mock.SentinelPolicy()
	in := []*structs.SentinelPolicy{policy1, policy2}

	// Test compilation
	prep, err := prepareSentinelPolicies(s1.sentinel, in)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(prep))
}

func TestSentinelResultToWarnErr(t *testing.T) {
	sent := sentinel.New(nil)

	// Setup three policies:
	// p1: Fails, Hard-mandatory (err)
	// p2: Fails, Soft-mandatory + Override (warn)
	// p3: Fails, Advisory (warn)
	p1 := mock.SentinelPolicy()
	p1.EnforcementLevel = structs.SentinelEnforcementLevelHardMandatory
	p1.Policy = "main = rule { false }"

	p2 := mock.SentinelPolicy()
	p2.EnforcementLevel = structs.SentinelEnforcementLevelSoftMandatory
	p2.Policy = "main = rule { false }"

	p3 := mock.SentinelPolicy()
	p3.EnforcementLevel = structs.SentinelEnforcementLevelAdvisory
	p3.Policy = "main = rule { false }"

	// Prepare the policies
	ps := []*structs.SentinelPolicy{p1, p2, p3}
	prep, err := prepareSentinelPolicies(sent, ps)
	assert.Nil(t, err)

	// Evaluate with an override
	result := sent.Eval(prep, &sentinel.EvalOpts{
		Override: true,
		EvalAll:  true, // For testing
	})

	// Get the errors
	warn, err := sentinelResultToWarnErr(result)
	assert.NotNil(t, err)
	assert.NotNil(t, warn)

	// Check the lengths
	assert.Equal(t, 1, len(err.(*multierror.Error).Errors))
	assert.Equal(t, 2, len(warn.(*multierror.Error).Errors))
}
