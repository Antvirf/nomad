//go:build ent
// +build ent

package nomad

import (
	"errors"
	"time"

	"github.com/armon/go-metrics"

	"github.com/hashicorp/nomad/nomad/structs"
)

// License endpoint is used for manipulating an enterprise license
type License struct {
	srv *Server
	ctx *RPCContext
}

func NewLicenseEndpoint(srv *Server, ctx *RPCContext) *License {
	return &License{srv: srv, ctx: ctx}
}

// COMPAT: License.UpsertLicense was deprecated in Nomad 1.1.0
// UpsertLicense is used to set an enterprise license
func (l *License) UpsertLicense(args *structs.LicenseUpsertRequest, reply *structs.GenericResponse) error {
	return errors.New("License.UpsertLicense is deprecated")
}

// GetLicense is used to retrieve an enterprise license
func (l *License) GetLicense(args *structs.LicenseGetRequest, reply *structs.LicenseGetResponse) error {
	authErr := l.srv.Authenticate(l.ctx, args)
	l.srv.MeasureRPCRate("license", structs.RateMetricRead, args)
	if authErr != nil {
		return structs.ErrPermissionDenied
	}
	defer metrics.MeasureSince([]string{"nomad", "license", "get_license"}, time.Now())

	// Check OperatorRead permissions
	if aclObj, err := l.srv.ResolveToken(args.AuthToken); err != nil {
		return err
	} else if aclObj != nil && !aclObj.AllowOperatorRead() {
		return structs.ErrPermissionDenied
	}

	out := l.srv.EnterpriseState.License()
	reply.NomadLicense = out
	reply.ConfigOutdated = l.srv.EnterpriseState.FileLicenseOutdated()

	return nil
}
