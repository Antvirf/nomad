// +build ent

package nomad

import (
	"strings"
	"testing"

	msgpackrpc "github.com/hashicorp/net-rpc-msgpackrpc"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
)

func TestJobEndpoint_Register_Sentinel(t *testing.T) {
	t.Parallel()
	s1, root := testACLServer(t, func(c *Config) {
		c.NumSchedulers = 0 // Prevent automatic dequeue
	})
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create a passing policy
	policy1 := mock.SentinelPolicy()
	policy1.EnforcementLevel = structs.SentinelEnforcementLevelHardMandatory
	s1.State().UpsertSentinelPolicies(1000,
		[]*structs.SentinelPolicy{policy1})

	// Create the register request
	job := mock.Job()
	req := &structs.JobRegisterRequest{
		Job: job,
		WriteRequest: structs.WriteRequest{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}

	// Should work
	var resp structs.JobRegisterResponse
	if err := msgpackrpc.CallWithCodec(codec, "Job.Register", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Create a failing policy
	policy2 := mock.SentinelPolicy()
	policy2.EnforcementLevel = structs.SentinelEnforcementLevelSoftMandatory
	policy2.Policy = "main = rule { false }"
	s1.State().UpsertSentinelPolicies(1001,
		[]*structs.SentinelPolicy{policy2})

	// Should fail
	if err := msgpackrpc.CallWithCodec(codec, "Job.Register", req, &resp); err == nil {
		t.Fatalf("expected error")
	}

	// Do the same request with an override set
	req.PolicyOverride = true

	// Should work, with a warning
	if err := msgpackrpc.CallWithCodec(codec, "Job.Register", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(resp.Warnings, policy2.Name) {
		t.Fatalf("bad: %s", resp.Warnings)
	}
}

func TestJobEndpoint_Register_Sentinel_DriverForce(t *testing.T) {
	t.Parallel()
	s1, root := testACLServer(t, func(c *Config) {
		c.NumSchedulers = 0 // Prevent automatic dequeue
	})
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create a passing policy
	policy1 := mock.SentinelPolicy()
	policy1.EnforcementLevel = structs.SentinelEnforcementLevelHardMandatory
	policy1.Policy = `
	main = rule { all_drivers_exec }

	all_drivers_exec = rule {
		all job.task_groups as tg {
			all tg.tasks as task {
				task.driver is "exec"
			}
		}
	}
	`
	s1.State().UpsertSentinelPolicies(1000,
		[]*structs.SentinelPolicy{policy1})

	// Create the register request
	job := mock.Job()
	req := &structs.JobRegisterRequest{
		Job: job,
		WriteRequest: structs.WriteRequest{
			Region:    "global",
			AuthToken: root.SecretID,
		},
	}

	// Should work
	var resp structs.JobRegisterResponse
	if err := msgpackrpc.CallWithCodec(codec, "Job.Register", req, &resp); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Create a failing job
	job2 := mock.Job()
	job2.TaskGroups[0].Tasks[0].Driver = "docker"
	req.Job = job2

	// Should fail
	if err := msgpackrpc.CallWithCodec(codec, "Job.Register", req, &resp); err == nil {
		t.Fatalf("expected error")
	}

	// Should fail even with override
	req.PolicyOverride = true
	if err := msgpackrpc.CallWithCodec(codec, "Job.Register", req, &resp); err == nil {
		t.Fatalf("expected error")
	}
}

func TestJobEndpoint_Plan_Sentinel(t *testing.T) {
	t.Parallel()
	s1, root := testACLServer(t, func(c *Config) {
		c.NumSchedulers = 0 // Prevent automatic dequeue
	})
	defer s1.Shutdown()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	// Create a passing policy
	policy1 := mock.SentinelPolicy()
	policy1.EnforcementLevel = structs.SentinelEnforcementLevelHardMandatory
	policy1.Policy = `
	main = rule { all_drivers_exec }

	all_drivers_exec = rule {
		all job.task_groups as tg {
			all tg.tasks as task {
				task.driver is "exec"
			}
		}
	}
	`
	s1.State().UpsertSentinelPolicies(1000,
		[]*structs.SentinelPolicy{policy1})

	// Create a plan request
	job := mock.Job()
	planReq := &structs.JobPlanRequest{
		Job:  job,
		Diff: true,
		WriteRequest: structs.WriteRequest{
			Region:    "global",
			Namespace: job.Namespace,
			AuthToken: root.SecretID,
		},
	}

	// Fetch the response
	var planResp structs.JobPlanResponse
	if err := msgpackrpc.CallWithCodec(codec, "Job.Plan", planReq, &planResp); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Create a failing job
	job2 := mock.Job()
	job2.TaskGroups[0].Tasks[0].Driver = "docker"
	planReq.Job = job2

	// Should fail
	if err := msgpackrpc.CallWithCodec(codec, "Job.Plan", planReq, &planResp); err == nil {
		t.Fatalf("expected error")
	}

	// Should fail, even with override
	planReq.PolicyOverride = true
	if err := msgpackrpc.CallWithCodec(codec, "Job.Plan", planReq, &planResp); err == nil {
		t.Fatalf("expected error")
	}
}
