// +build ent

package quotas

import (
	"os"

	e2e "github.com/hashicorp/nomad/e2e/e2eutil"
	"github.com/hashicorp/nomad/e2e/framework"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/jobspec"
)

type QuotasE2ETest struct {
	framework.TC
	namespaceIDs     []string
	namespacedJobIDs [][2]string // [(ns, jobID)]
	quotaIDs         []string
}

func init() {
	framework.AddSuites(&framework.TestSuite{
		Component:   "Quotas",
		CanRunLocal: true,
		Consul:      true,
		Cases: []framework.TestCase{
			new(QuotasE2ETest),
		},
	})

}

func (tc *QuotasE2ETest) BeforeAll(f *framework.F) {
	e2e.WaitForLeader(f.T(), tc.Nomad())
	e2e.WaitForNodesReady(f.T(), tc.Nomad(), 1)
}

func (tc *QuotasE2ETest) AfterEach(f *framework.F) {
	if os.Getenv("NOMAD_TEST_SKIPCLEANUP") == "1" {
		return
	}

	for _, pair := range tc.namespacedJobIDs {
		ns := pair[0]
		jobID := pair[1]
		if ns != "" {
			_, err := e2e.Command("nomad", "job", "stop", "-purge", "-namespace", ns, jobID)
			f.Assert().NoError(err)
		} else {
			_, err := e2e.Command("nomad", "job", "stop", "-purge", jobID)
			f.Assert().NoError(err)
		}
	}
	tc.namespacedJobIDs = [][2]string{}

	for _, ns := range tc.namespaceIDs {
		_, err := e2e.Command("nomad", "namespace", "delete", ns)
		f.Assert().NoError(err)
	}
	tc.namespaceIDs = []string{}

	for _, quota := range tc.quotaIDs {
		_, err := e2e.Command("nomad", "quota", "delete", quota)
		f.Assert().NoError(err)
	}
	tc.quotaIDs = []string{}

	_, err := e2e.Command("nomad", "system", "gc")
	f.Assert().NoError(err)
}

func (tc *QuotasE2ETest) quota(name, path string) error {
	_, err := e2e.Command("nomad", "quota", "apply", path)
	tc.quotaIDs = append(tc.quotaIDs, name)
	return err
}

// TestQuotasIncreasedCount adds resources to a job with an existing quota
// until it fails
func (tc *QuotasE2ETest) TestQuotasIncreasedCount(f *framework.F) {

	f.NoError(tc.quota("quotaA", "quotas/input/quota_a.hcl"))

	_, err := e2e.Command("nomad", "namespace", "apply",
		"-quota", "quotaA",
		"-description", "namespace A", "NamespaceA")
	f.NoError(err, "could not create namespace")
	tc.namespaceIDs = append(tc.namespaceIDs, "NamespaceA")

	jobA := "test-quotas-a-" + uuid.Generate()[0:8]
	tc.namespacedJobIDs = append(tc.namespacedJobIDs, [2]string{"NamespaceA", jobA})

	// for the initial registration, we want a lower count so we fit
	job, err := jobspec.ParseFile("quotas/input/job_a.nomad")
	f.NoError(err)
	job.ID = &jobA
	count := 1
	job.TaskGroups[0].Count = &count
	_, _, err = tc.Nomad().Jobs().Register(job, nil)
	f.NoError(err, "could not register jobA")
	expected := []string{"running"}
	f.NoError(e2e.WaitForAllocStatusExpected(jobA, "NamespaceA", expected), "job should be running")

	// update the job, but still fit within the quota
	count = 2
	job.TaskGroups[0].Count = &count
	_, _, err = tc.Nomad().Jobs().Register(job, nil)
	f.NoError(err, "could not register jobA")
	expected = []string{"running", "running"}
	f.NoError(e2e.WaitForAllocStatusExpected(jobA, "NamespaceA", expected), "job should be running")

	// increase above the quota
	err = e2e.Register(jobA, "quotas/input/job_a.nomad")
	f.Contains(err.Error(), `Task Group "group" (failed to place 1 allocation)`)
	f.Contains(err.Error(), `* Quota limit hit "memory exhausted (384 needed > 300 limit)`)
}

// TestQuotasAddedLater adds quotas to a namespace with an existing job
func (tc *QuotasE2ETest) TestQuotasAddedLater(f *framework.F) {
	f.NoError(tc.quota("quotaB", "quotas/input/quota_b.hcl"))

	// no quota on namespace to start
	_, err := e2e.Command("nomad", "namespace", "apply",
		"-description", "namespace B", "NamespaceB")
	f.NoError(err, "could not create namespace")
	tc.namespaceIDs = append(tc.namespaceIDs, "NamespaceB")

	jobB := "test-quotas-b-" + uuid.Generate()[0:8]
	tc.namespacedJobIDs = append(tc.namespacedJobIDs, [2]string{"NamespaceB", jobB})

	// for the initial registration, we want a different env var
	job, err := jobspec.ParseFile("quotas/input/job_b.nomad")
	f.NoError(err)
	job.ID = &jobB
	job.TaskGroups[0].Tasks[0].Env["TEST"] = "Z"
	_, _, err = tc.Nomad().Jobs().Register(job, nil)
	f.NoError(err, "could not register jobB")
	expected := []string{"running", "running"}
	f.NoError(e2e.WaitForAllocStatusExpected(jobB, "NamespaceB", expected), "job should be running")

	// apply the quota
	_, err = e2e.Command("nomad", "namespace", "apply",
		"-quota", "quotaB",
		"-description", "namespace B", "NamespaceB")
	f.NoError(err, "could not apply quota to namespace")

	err = e2e.Register(jobB, "quotas/input/job_b.nomad")
	f.Contains(err.Error(), `Task Group "group" (failed to place 1 allocation)`)
	f.Contains(err.Error(), `* Quota limit hit "cpu exhausted (512 needed > 300 limit)`)

	// query the quota status and get expected errors for invalid region
	out, err := e2e.Command("nomad", "quota", "status", "quotaB")
	f.Error(err, "exit status 1")

	section, err := e2e.GetSection(out, "Quota Limits")
	f.NoError(err, "could not find Quota Limits section")

	rows, err := e2e.ParseColumns(section)
	for _, row := range rows {
		if row["Region"] == "global" {
			f.Equal("512 / 300", row["CPU Usage"])
		}
	}

	section, err = e2e.GetSection(out, "Lookup Failures")
	f.NoError(err, "could not find Lookup Failures section")
	f.Contains(section, "No path to region")
}

// TestQuotasBetweenJobs adds jobs to a namespace with quotas until it fails
func (tc *QuotasE2ETest) TestQuotasBetweenJobs(f *framework.F) {

	f.NoError(tc.quota("quotaC", "quotas/input/quota_c.hcl"))

	_, err := e2e.Command("nomad", "namespace", "apply",
		"-quota", "quotaC",
		"-description", "namespace C", "NamespaceC")
	f.NoError(err, "could not create namespace")
	tc.namespaceIDs = append(tc.namespaceIDs, "NamespaceC")

	// 1st job fits
	jobC1 := "test-quotas-c-" + uuid.Generate()[0:8]
	tc.namespacedJobIDs = append(tc.namespacedJobIDs, [2]string{"NamespaceC", jobC1})
	err = e2e.Register(jobC1, "quotas/input/job_c.nomad")
	f.NoError(err, "could not register jobC1")
	expected := []string{"running"}
	f.NoError(e2e.WaitForAllocStatusExpected(jobC1, "NamespaceC", expected), "job should be running")

	// 2nd job should not
	jobC2 := "test-quotas-c-" + uuid.Generate()[0:8]
	tc.namespacedJobIDs = append(tc.namespacedJobIDs, [2]string{"NamespaceC", jobC2})
	err = e2e.Register(jobC2, "quotas/input/job_c.nomad")
	f.Contains(err.Error(), `Task Group "group" (failed to place 1 allocation)`)
	f.Contains(err.Error(), `* Quota limit hit "memory exhausted (256 needed > 200 limit)`)
}
