// +build ent

package nomad

import (
	"fmt"
	"time"

	metrics "github.com/armon/go-metrics"
	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/nomad/nomad/structs"
)

// License endpoint is used for manipulating an enterprise license
type License struct {
	srv *Server
}

var minLicenseSetVersion = version.Must(version.NewVersion("0.12.0"))

// UpsertLicense is used to set an enterprise license
func (l *License) UpsertLicense(args *structs.LicenseUpsertRequest, reply *structs.GenericResponse) error {
	if done, err := l.srv.forward("License.UpsertLicense", args, args, reply); done {
		return err
	}

	// Check OperatorWrite permissions
	if aclObj, err := l.srv.ResolveToken(args.AuthToken); err != nil {
		return err
	} else if aclObj != nil && !aclObj.AllowOperatorWrite() {
		return structs.ErrPermissionDenied
	}

	// Ensure all servers meet minimum requirements
	if !ServersMeetMinimumVersion(l.srv.Members(), minLicenseSetVersion, false) {
		l.srv.logger.Warn("cannot set license until all servers are above minimum version", "min_version", minLicenseMetaVersion)
		return fmt.Errorf("all servers do not meet minimum version requirement: %s", minLicenseMetaVersion)
	}

	// Validate license pre-upsert
	if _, err := l.srv.EnterpriseState.licenseWatcher.ValidateLicense(args.License.Signed); err != nil {
		return structs.NewErrRPCCoded(400, fmt.Sprintf("error validating license: %s", err.Error()))
	}

	defer metrics.MeasureSince([]string{"nomad", "license", "upsert_license"}, time.Now())

	// Update via Raft
	out, index, err := l.srv.raftApply(structs.LicenseUpsertRequestType, args)
	if err != nil {
		return err
	}

	// Check if there was an error when applying
	if err, ok := out.(error); ok && err != nil {
		return err
	}

	// Update the index
	reply.Index = index

	return nil
}

// GetLicense is used to retrieve an enterprise license
func (l *License) GetLicense(args *structs.LicenseGetRequest, reply *structs.LicenseGetResponse) error {
	if done, err := l.srv.forward("License.GetLicense", args, args, reply); done {
		return err
	}

	// Check OperatorRead permissions
	if aclObj, err := l.srv.ResolveToken(args.AuthToken); err != nil {
		return err
	} else if aclObj != nil && !aclObj.AllowOperatorRead() {
		return structs.ErrPermissionDenied
	}

	defer metrics.MeasureSince([]string{"nomad", "license", "get_license"}, time.Now())

	// Fetch license existing in Watcher
	out := l.srv.EnterpriseState.licenseWatcher.License()
	reply.NomadLicense = out
	return nil
}
