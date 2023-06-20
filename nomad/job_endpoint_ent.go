//go:build ent
// +build ent

package nomad

import (
	"fmt"
	"strings"
	"time"

	memdb "github.com/hashicorp/go-memdb"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad-licensing/license"
	"github.com/hashicorp/nomad/nomad/structs"
)

// enforceSubmitJob is used to check any Sentinel policies for the submit-job scope
func (j *Job) enforceSubmitJob(override bool, job *structs.Job, nomadACLToken *structs.ACLToken, ns *structs.Namespace) (error, error) {
	dataCB := func() map[string]interface{} {
		return map[string]interface{}{
			"job":             job,
			"nomad_acl_token": nomadACLToken,
			"namespace":       ns,
		}
	}
	return j.srv.enforceScope(override, structs.SentinelScopeSubmitJob, dataCB)
}

// multiregionCreateDeployment returns a deployment that is created when a
// multiregion job is registered.
//
// Without this extra information it is impossible, in some cases, to
// distinguish between a local change (such as a node update) from a remote one
// (such as a job update). The scheduler needs to know this difference to
// generate the right number of placements: multiregion deployments do not
// place allocations until another region unblocks them, while local changes
// must place allocations immediately since there is no other region involved.
func (j *Job) multiregionCreateDeployment(job *structs.Job, eval *structs.Evaluation) *structs.Deployment {
	if !job.IsMultiregion() || !job.UsesDeployments() {
		return nil
	}

	deployment := structs.NewDeployment(job, eval.Priority)
	deployment.Status = structs.DeploymentStatusInitializing
	deployment.StatusDescription = structs.DeploymentStatusDescriptionPendingForPeer

	return deployment
}

// interpolateMultiregionFields interpolates a job for a specific region
func (j *Job) interpolateMultiregionFields(args *structs.JobPlanRequest) error {

	// a multiregion job that's been interpolated for fan-out will never
	// have the "global" region.
	if args.Job.Region != "global" || args.Job.Multiregion == nil {
		return nil
	}

	// enterprise license enforcement - if not licensed then users can't
	// plan multiregion jobs.
	err := j.srv.EnterpriseState.FeatureCheck(license.FeatureMultiregionDeployments, true)
	if err != nil {
		return err
	}

	for _, region := range args.Job.Multiregion.Regions {
		if region.Name == j.srv.Region() {
			args.Job = regionalJob(args.Job, region)
			break
		}
	}
	return nil
}

// multiregionRegister is used to send a job across multiple regions.  The
// bool returned is a flag indicating if the caller's region is the "runner",
// which kicks off the deployments of peer regions in `multiregionStart`.
func (j *Job) multiregionRegister(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse, newVersion uint64) (bool, error) {

	// a multiregion job that's been interpolated for fan-out will never
	// have the "global" region.
	if args.Job.Region != "global" || args.Job.Multiregion == nil {
		return false, nil
	}

	// enterprise license enforcement - if not licensed then users can't
	// register/update multiregion jobs.
	err := j.srv.EnterpriseState.FeatureCheck(license.FeatureMultiregionDeployments, true)
	if err != nil {
		return false, err
	}

	var isRunner bool
	requests := []*structs.JobRegisterRequest{}

	var job *structs.Job

	for _, region := range args.Job.Multiregion.Regions {
		if region.Name == j.srv.Region() {
			job = regionalJob(args.Job.Copy(), region)
			isRunner = true
		} else {
			existed, remoteVersion, jobModifyIndex, err := j.getJobVersion(args, region.Name)
			if err != nil {
				return false, err
			}
			// versions must increase, but checking 'existed' lets us avoid
			// having the first version of a MRD job always be 1.
			if existed {
				if newVersion <= remoteVersion {
					newVersion = remoteVersion + 1
				}
			}
			req := *args // copies everything except the job
			req.Job = regionalJob(args.Job.Copy(), region)
			req.Region = region.Name
			req.EnforceIndex = true
			req.JobModifyIndex = jobModifyIndex
			requests = append(requests, &req)
		}
	}

	// in a multiregion deployment, the RPC-receiving region must be one of the
	// regions in the deployment, so this case violates our invariants.
	if job == nil {
		return false, fmt.Errorf(
			"could not register job %q: rpc forwarded to region %q where the job was not submitted",
			args.Job.ID, j.srv.Region())
	}

	args.Job = job
	args.Job.Version = newVersion
	warnings := []string{}

	for _, req := range requests {
		req.Job.Version = newVersion
		resp := &structs.JobRegisterResponse{}
		err := j.Register(req, resp)
		if resp.Warnings != "" {
			warnings = append(warnings, resp.Warnings)
		}
		if err != nil {
			reply.Warnings = strings.Join(warnings, "\n")
			return false, fmt.Errorf("could not register job %q in region %q: %w",
				req.Job.ID, req.Region, err)
		}
	}
	reply.Warnings = strings.Join(warnings, "\n")
	return isRunner, nil
}

// getJobVersion gets the job version and modify index for the job from a
// specific region, as well as whether the job existed in that region (to
// differentiate the default value for Version)
func (j *Job) getJobVersion(args *structs.JobRegisterRequest, region string) (bool, uint64, uint64, error) {
	req := &structs.JobSpecificRequest{
		JobID: args.Job.ID,
		QueryOptions: structs.QueryOptions{
			Namespace: args.Namespace,
			Region:    region,
			AuthToken: args.AuthToken,
		},
	}
	resp := &structs.SingleJobResponse{}
	err := j.GetJob(req, resp)
	if err != nil {
		return false, 0, 0, err
	}
	if resp.Job == nil {
		return false, 0, 0, nil
	}
	return true, resp.Job.Version, resp.Job.JobModifyIndex, nil
}

// regionalJob interpolates a multiregion job for a specific region
func regionalJob(j *structs.Job, region *structs.MultiregionRegion) *structs.Job {
	j.Region = region.Name
	if len(region.Datacenters) != 0 {
		j.Datacenters = region.Datacenters
	}
	if region.NodePool != "" {
		j.NodePool = region.NodePool
	}

	for _, tg := range j.TaskGroups {
		if tg.Count == 0 {
			tg.Count = region.Count
		}
	}

	// Override the job meta with the region meta. The job meta doesn't
	// get merged with the group/task meta until it lands on the client.
	for k, v := range region.Meta {
		if j.Meta == nil {
			j.Meta = map[string]string{}
		}
		j.Meta[k] = v
	}
	return j
}

// multiregionStart is used to kick-off the deployment across multiple regions
func (j *Job) multiregionStart(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse) error {
	if !args.Job.UsesDeployments() {
		return nil
	}

	// by this point we've been interpolated for fan-out
	if args.Job.Multiregion == nil {
		return nil
	}

	// enterprise license enforcement - if not licensed then users can't
	// register/update multiregion jobs.
	err := j.srv.EnterpriseState.FeatureCheck(license.FeatureMultiregionDeployments, true)
	if err != nil {
		return err
	}

	job := args.Job
	runReqs := []*structs.DeploymentRunRequest{}

	version, err := j.versionForModifyIndex(job.Namespace, job.ID, args.Job.JobModifyIndex)
	if err != nil {
		return err
	}

	// collect all requests to run the region first to ensure that all regions
	// created a deployment before starting any one of them
	for _, region := range args.Job.Multiregion.Regions {
		if jobIsMultiregionStarter(job, region.Name) {
			req := &structs.JobSpecificRequest{
				JobID: job.ID,
				QueryOptions: structs.QueryOptions{
					Region:    region.Name,
					Namespace: job.Namespace,
					AuthToken: args.AuthToken,
				},
			}
			deploymentID, err := j.deploymentIDForJobVersion(req, version)
			if err != nil {
				return fmt.Errorf("could not find deployment for job %q in region %q: %w",
					job.ID, region.Name, err)
			}

			runReq := &structs.DeploymentRunRequest{
				DeploymentID: deploymentID,
				WriteRequest: structs.WriteRequest{
					Region:    region.Name,
					Namespace: job.Namespace,
					AuthToken: args.AuthToken,
				},
			}
			runReqs = append(runReqs, runReq)
		}
	}

	if args.Job.Multiregion.Strategy == nil ||
		args.Job.Multiregion.Strategy.MaxParallel == 0 {
		var mErr multierror.Error
		for _, req := range runReqs {
			err = j.srv.RPC("Deployment.Run", req, &structs.DeploymentUpdateResponse{})
			if err != nil {
				multierror.Append(&mErr, err)
			}
		}
		return mErr.ErrorOrNil()
	}

	for _, req := range runReqs {
		err = j.srv.RPC("Deployment.Run", req, &structs.DeploymentUpdateResponse{})
		if err != nil {
			return fmt.Errorf("could not start deployment for job %q in region %q: %w",
				job.ID, req.Region, err)
		}
	}

	return nil
}

// multiregionDrop is used to deregister regions from a previous version of the
// job that are no longer in use
func (j *Job) multiregionDrop(args *structs.JobRegisterRequest, reply *structs.JobRegisterResponse) error {

	// by this point we've been interpolated for fan-out
	if args.Job.Multiregion == nil {
		return nil
	}

	// Lookup the previous version of the job
	snap, err := j.srv.State().Snapshot()
	if err != nil {
		return err
	}
	ws := memdb.NewWatchSet()
	versions, err := snap.JobVersionsByID(ws, args.RequestNamespace(), args.Job.ID)
	if err != nil {
		return err
	}

	// find the first version that's not this version
	var existingJob *structs.Job
	for _, version := range versions {
		if version.Version != args.Job.Version {
			existingJob = version
		}
	}
	if existingJob == nil || existingJob.Multiregion == nil {
		return nil
	}

	// enterprise license enforcement - if not licensed then users can't
	// register/update multiregion jobs.
	err = j.srv.EnterpriseState.FeatureCheck(license.FeatureMultiregionDeployments, true)
	if err != nil {
		return err
	}

	newRegions := map[string]bool{}
	for _, region := range args.Job.Multiregion.Regions {
		newRegions[region.Name] = true
	}

	return j.deregisterRegionImpl(existingJob, args.AuthToken, false,
		func(region *structs.MultiregionRegion) bool {
			_, ok := newRegions[region.Name]
			return !ok
		})
}

// multiregionStop is used to fan-out Job.Deregister RPCs to all regions if
// the global flag is passed to Job.Deregister
func (j *Job) multiregionStop(job *structs.Job, args *structs.JobDeregisterRequest, reply *structs.JobDeregisterResponse) error {

	if job == nil || job.Multiregion == nil {
		return nil
	}
	if !args.Global {
		return nil
	}

	return j.deregisterRegionImpl(job, args.AuthToken, args.Purge,
		func(region *structs.MultiregionRegion) bool {
			return region.Name != j.srv.Region()
		})
}

// deregisterRegionImpl fans-out non-global Job.Deregister RPCs to all regions
// that meet the 'test' function.
func (j *Job) deregisterRegionImpl(job *structs.Job, authToken string, purge bool, test func(*structs.MultiregionRegion) bool) error {

	deleteReqs := []*structs.JobDeregisterRequest{}

	for _, region := range job.Multiregion.Regions {
		if test(region) {
			deleteReqs = append(deleteReqs,
				&structs.JobDeregisterRequest{
					JobID:  job.ID,
					Purge:  purge,
					Global: false, // set explicitly to call attention to it
					WriteRequest: structs.WriteRequest{
						Region:    region.Name,
						Namespace: job.Namespace,
						AuthToken: authToken,
					},
				},
			)
		}
	}

	var mErr multierror.Error
	for _, req := range deleteReqs {
		err := j.srv.RPC("Job.Deregister", req, &structs.JobDeregisterResponse{})
		if err != nil {
			multierror.Append(&mErr, err)
		}
	}
	return mErr.ErrorOrNil()
}

// versionForModifyIndex finds the job version associated with a given
// modifyIndex. we know all regions will have the same version, but we
// don't know what it is because the fsm apply may have coerced it to
// 0. we know the JobModifyIndex so use that to get the Version out of the
// state store.  in the typical case the first Job version we look at will
// be the right one but we need to check in case of concurrent updates
func (j *Job) versionForModifyIndex(namespace, jobID string, modifyIndex uint64) (uint64, error) {
	snap, err := j.srv.fsm.State().Snapshot()
	if err != nil {
		return 0, err
	}
	ws := memdb.NewWatchSet()
	allVersions, err := snap.JobVersionsByID(ws, namespace, jobID)
	if err != nil {
		return 0, err
	}
	for _, jobVersion := range allVersions {
		if jobVersion.JobModifyIndex == modifyIndex {
			return jobVersion.Version, nil
		}
	}
	return 0, fmt.Errorf(
		"could not find version for job %q with modify index %d", jobID, modifyIndex)
}

// deploymentForJobVersion queries the remote region for the deployment ID that
// has been initialized with retries and backoff. If we don't have a deployment
// within the time window then the job is considered to have failed to schedule.
// You're likely to end up with regions stuck in a "pending" state
func (j *Job) deploymentIDForJobVersion(req *structs.JobSpecificRequest, version uint64) (string, error) {
	reply := &structs.DeploymentListResponse{}
	retry := 0
	for {
		err := j.Deployments(req, reply)
		if err != nil {
			return "", err
		}
		for _, deployment := range reply.Deployments {
			if deployment.JobVersion == version && deployment.Status == structs.DeploymentStatusPending {
				return deployment.ID, nil
			}
		}
		// we'll retry with backoffs for a bit over 100s
		if retry > 10 {
			return "", fmt.Errorf("timed out waiting for deployment")
		}
		backoff := 1 << retry * 100 * time.Millisecond
		retry++
		time.Sleep(backoff)
	}
}

// jobIsMultiregionStarter returns whether a regional job should begin
// in the running state
func jobIsMultiregionStarter(j *structs.Job, regionName string) bool {
	if !j.IsMultiregion() {
		return true
	}
	if j.Type == "system" || j.Type == "batch" {
		return true
	}
	if j.Multiregion.Strategy == nil || j.Multiregion.Strategy.MaxParallel == 0 {
		return true
	}
	for i, region := range j.Multiregion.Regions {
		if region.Name == regionName {
			return i < j.Multiregion.Strategy.MaxParallel
		}
	}
	return false
}

// multiregionSpecChanged checks to see if the job spec has changed. If the job is multiregion,
// it checks all regions to determine if any deployed jobs instances have been stopped or
// otherwise differ from the incoming jobspec. Since multiregion jobs require coordinated
// deployments and synchronized job versions across all regions, a change in one requires
// redeployment of all.
func (j *Job) multiregionSpecChanged(existingJob *structs.Job, args *structs.JobRegisterRequest) (bool, error) {
	if existingJob.SpecChanged(args.Job) {
		return true, nil
	}

	if !existingJob.IsMultiregion() {
		return false, nil
	}

	// Copy the job so that we can mutate it by region and compare it with the response.
	incomingJob := args.Job.Copy()

	// If the local region's spec hasn't changed, but this is a multiregion job, check other regions.
	for _, region := range existingJob.Multiregion.Regions {
		if region.Name == args.RequestRegion() {
			continue
		}

		incomingJob.Region = region.Name

		req := &structs.JobSpecificRequest{
			JobID: incomingJob.ID,
			QueryOptions: structs.QueryOptions{
				Region:    region.Name,
				Namespace: incomingJob.Namespace,
				AuthToken: args.AuthToken,
			},
		}

		resp := structs.SingleJobResponse{}

		err := j.GetJob(req, &resp)
		if err != nil {
			j.logger.Error("multiregion job lookup failed", "job_id", incomingJob.ID, "region", incomingJob.Region, "namespace", incomingJob.Namespace, "error", err)
			return false, err
		}

		// If the region's jobspec changed or is nil (purged) for that region, assume it's being re-registered
		// and treat like a spec change.
		if resp.Job == nil || resp.Job.SpecChanged(incomingJob) {
			return true, nil
		}
	}

	return false, nil
}
