//go:build ent
// +build ent

package nomad

import (
	"testing"

	"github.com/hashicorp/nomad/ci"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
	"github.com/shoenig/test/must"
)

func TestJobEndpointHook_VaultEnt(t *testing.T) {
	ci.Parallel(t)

	srv, cleanup := TestServer(t, func(c *Config) {
		c.NumSchedulers = 0
	})
	t.Cleanup(cleanup)
	testutil.WaitForLeader(t, srv.RPC)

	job := mock.Job()

	// create two different Vault blocks and assign to clusters
	job.TaskGroups[0].Tasks = append(job.TaskGroups[0].Tasks, job.TaskGroups[0].Tasks[0].Copy())
	job.TaskGroups[0].Tasks[0].Vault = &structs.Vault{Cluster: "default"}
	job.TaskGroups[0].Tasks[1].Name = "web2"
	job.TaskGroups[0].Tasks[1].Vault = &structs.Vault{Cluster: "infra"}

	cases := []struct {
		name         string
		cfg          *structs.NamespaceVaultConfiguration
		expectVault0 string
		expectVault1 string
		expectError  string
	}{
		{
			name:         "no vault config",
			expectVault0: "default",
			expectVault1: "infra",
		},
		{
			name: "has allowed set",
			cfg: &structs.NamespaceVaultConfiguration{
				Default: "a-default",
				Allowed: []string{"a-default", "default", "infra"},
			},
			expectVault0: "default",
			expectVault1: "infra",
		},
		{
			name: "not in allowed set",
			cfg: &structs.NamespaceVaultConfiguration{
				Default: "a-default",
				Allowed: []string{"a-default", "infra"},
			},
			expectVault0: "default",
			expectVault1: "infra",
			expectError:  `namespace "default" does not allow jobs to use vault cluster "default"`,
		},
		{
			name: "has denied set",
			cfg: &structs.NamespaceVaultConfiguration{
				Default: "a-default",
				Denied:  []string{"infra", "default"},
			},
			expectVault0: "default",
			expectVault1: "infra",
			expectError: `2 errors occurred:
	* namespace "default" does not allow jobs to use vault cluster "default"
	* namespace "default" does not allow jobs to use vault cluster "infra"

`,
		},
		{
			name: "default in denied set is allowed",
			cfg: &structs.NamespaceVaultConfiguration{
				Default: "default",
				Denied:  []string{"infra", "default"},
			},
			expectVault0: "default",
			expectVault1: "infra",
			expectError:  `namespace "default" does not allow jobs to use vault cluster "infra"`,
		},
		{
			name: "empty allow list denies all",
			cfg: &structs.NamespaceVaultConfiguration{
				Default: "default",
				Allowed: []string{},
			},
			expectVault0: "default",
			expectVault1: "infra",
			expectError:  `namespace "default" does not allow jobs to use vault cluster "infra"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			job := job.Copy()

			ns := mock.Namespace()
			ns.Name = job.Namespace
			ns.VaultConfiguration = tc.cfg
			ns.SetHash()
			srv.fsm.State().UpsertNamespaces(1000, []*structs.Namespace{ns})

			hook := jobVaultHook{srv}
			_, _, err := hook.Mutate(job)
			must.NoError(t, err)
			must.Eq(t, tc.expectVault0, job.TaskGroups[0].Tasks[0].Vault.Cluster)
			must.Eq(t, tc.expectVault1, job.TaskGroups[0].Tasks[1].Vault.Cluster)

			// skipping over the rest of Validate b/c it requires an actual
			// Vault cluster
			err = hook.validateClustersForNamespace(job, job.Vault())
			if tc.expectError != "" {
				must.EqError(t, err, tc.expectError)
			} else {
				must.NoError(t, err)
			}
		})
	}

}
