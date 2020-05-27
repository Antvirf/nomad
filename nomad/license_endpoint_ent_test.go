// +build ent

package nomad

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/go-licensing"
	msgpackrpc "github.com/hashicorp/net-rpc-msgpackrpc"
	nomadLicense "github.com/hashicorp/nomad-licensing/license"
	"github.com/hashicorp/nomad/acl"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLicenseEndpoint_GetLicense(t *testing.T) {
	t.Parallel()
	TestLicenseValidationHelper(t)

	s1, cleanupS1 := TestServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	l := nomadLicense.NewTestLicense(nomadLicense.TestGovernancePolicyFlags())
	_, err := s1.EnterpriseState.licenseWatcher.SetLicense(l.Signed)
	require.NoError(t, err)

	// There is some time between SetLicense and the watchers updateCh
	// receiving and applying the new license
	testutil.WaitForResult(func() (bool, error) {
		get := &structs.LicenseGetRequest{
			QueryOptions: structs.QueryOptions{Region: "global"},
		}

		var resp structs.LicenseGetResponse
		require.NoError(t, msgpackrpc.CallWithCodec(codec, "License.GetLicense", get, &resp))

		equal := l.License.License.Equal(resp.NomadLicense.License)
		if equal {
			return true, nil
		}
		return false, fmt.Errorf("wanted: %v got: %v", l.License.License, resp.NomadLicense.License)
	}, func(err error) {
		require.Failf(t, "failed to find updated license", err.Error())
	})

}

func TestLicenseEndpoint_UpsertLicense(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	TestLicenseValidationHelper(t)

	s1, cleanupS1 := TestServer(t, nil)
	defer cleanupS1()

	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	now := time.Now()
	exp := 1 * time.Hour
	// Create a new license to upsert
	putLicense := &licensing.License{
		LicenseID:       "new-temp-license",
		CustomerID:      "temporary license customer",
		InstallationID:  "*",
		Product:         nomadLicense.ProductName,
		IssueTime:       now,
		StartTime:       now,
		ExpirationTime:  now.Add(exp),
		TerminationTime: now.Add(exp),
		Flags:           nomadLicense.TestGovernancePolicyFlags(),
	}

	putSigned, err := putLicense.SignedString(nomadLicense.TestPrivateKey)
	require.NoError(t, err)

	req := &structs.LicenseUpsertRequest{
		License:      &structs.StoredLicense{Signed: putSigned},
		WriteRequest: structs.WriteRequest{Region: "global"},
	}
	var resp structs.GenericResponse
	require.NoError(t, msgpackrpc.CallWithCodec(codec, "License.UpsertLicense", req, &resp))
	assert.NotEqual(uint64(0), resp.Index)

	// Check we created the license
	out, err := s1.fsm.State().License(nil)
	require.NoError(t, err)
	assert.Equal(out.Signed, putSigned)
}

func TestLicenseEndpoint_UpsertLicenses_ACL(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	TestLicenseValidationHelper(t)

	s1, root, cleanupS1 := TestACLServer(t, nil)
	defer cleanupS1()
	codec := rpcClient(t, s1)
	testutil.WaitForLeader(t, s1.RPC)

	now := time.Now()
	exp := 1 * time.Hour
	// Create a new license to upsert
	putLicense := &licensing.License{
		LicenseID:       "new-temp-license",
		CustomerID:      "temporary license customer",
		InstallationID:  "*",
		Product:         nomadLicense.ProductName,
		IssueTime:       now,
		StartTime:       now,
		ExpirationTime:  now.Add(exp),
		TerminationTime: now.Add(exp),
		Flags:           nomadLicense.TestGovernancePolicyFlags(),
	}

	putSigned, err := putLicense.SignedString(nomadLicense.TestPrivateKey)
	require.NoError(t, err)
	stored, _ := mock.StoredLicense()
	stored.Signed = putSigned

	state := s1.fsm.State()

	// Create the token
	invalidToken := mock.CreateToken(t, state, 1003, []string{"test-invalid", acl.PolicyWrite})

	// Create the register request
	req := &structs.LicenseUpsertRequest{
		License:      stored,
		WriteRequest: structs.WriteRequest{Region: "global"},
	}

	// Upsert the license without a token and expect failure
	{
		var resp structs.GenericResponse
		err := msgpackrpc.CallWithCodec(codec, "License.UpsertLicense", req, &resp)
		assert.NotNil(err)
		assert.Equal(err.Error(), structs.ErrPermissionDenied.Error())

		// Check we did not create the namespaces
		out, err := s1.fsm.State().License(nil)
		require.NoError(t, err)
		assert.Nil(out)
	}

	// Try with an invalid token
	req.AuthToken = invalidToken.SecretID
	{
		var resp structs.GenericResponse
		err := msgpackrpc.CallWithCodec(codec, "License.UpsertLicense", req, &resp)
		assert.NotNil(err)
		assert.Equal(err.Error(), structs.ErrPermissionDenied.Error())

		// Check we did not create the namespaces
		out, err := s1.fsm.State().License(nil)
		assert.Nil(err)
		assert.Nil(out)

	}

	// Try with a root token
	req.AuthToken = root.SecretID
	{
		var resp structs.GenericResponse
		assert.Nil(msgpackrpc.CallWithCodec(codec, "License.UpsertLicense", req, &resp))
		assert.NotEqual(uint64(0), resp.Index)

		// Check we created the namespaces
		out, err := s1.fsm.State().License(nil)
		require.NoError(t, err)
		assert.NotNil(out)
	}
}
