// +build pro ent

package nomad

import (
	"sort"
	"testing"
	"time"

	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/state"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/testutil"
	"github.com/stretchr/testify/assert"
)

func TestLeader_ReplicateNamespaces(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	s1, root := testACLServer(t, func(c *Config) {
		c.Region = "region1"
		c.AuthoritativeRegion = "region1"
		c.ACLEnabled = true
	})
	defer s1.Shutdown()
	s2, _ := testACLServer(t, func(c *Config) {
		c.Region = "region2"
		c.AuthoritativeRegion = "region1"
		c.ACLEnabled = true
		c.ReplicationBackoff = 20 * time.Millisecond
		c.ReplicationToken = root.SecretID
	})
	defer s2.Shutdown()
	testJoin(t, s1, s2)
	testutil.WaitForLeader(t, s1.RPC)
	testutil.WaitForLeader(t, s2.RPC)

	// Write a namespace to the authoritative region
	ns1 := mock.Namespace()
	assert.Nil(s1.State().UpsertNamespaces(100, []*structs.Namespace{ns1}))

	// Wait for the namespace to replicate
	testutil.WaitForResult(func() (bool, error) {
		state := s2.State()
		out, err := state.NamespaceByName(nil, ns1.Name)
		return out != nil, err
	}, func(err error) {
		t.Fatalf("should replicate namespace")
	})

	// Delete the namespace at the authoritative region
	assert.Nil(s1.State().DeleteNamespaces(200, []string{ns1.Name}))

	// Wait for the namespace deletion to replicate
	testutil.WaitForResult(func() (bool, error) {
		state := s2.State()
		out, err := state.NamespaceByName(nil, ns1.Name)
		return out == nil, err
	}, func(err error) {
		t.Fatalf("should replicate namespace deletion")
	})
}

func TestLeader_DiffNamespaces(t *testing.T) {
	t.Parallel()

	state := state.TestStateStore(t)

	// Populate the local state
	ns1 := mock.Namespace()
	ns2 := mock.Namespace()
	ns3 := mock.Namespace()
	assert.Nil(t, state.UpsertNamespaces(100, []*structs.Namespace{ns1, ns2, ns3}))

	// Simulate a remote list
	rns2 := ns2.Copy()
	rns2.ModifyIndex = 50 // Ignored, same index
	rns3 := ns3.Copy()
	rns3.ModifyIndex = 100 // Updated, higher index
	rns3.Hash = []byte{0, 1, 2, 3}
	ns4 := mock.Namespace()
	remoteList := []*structs.Namespace{
		rns2,
		rns3,
		ns4,
	}
	delete, update := diffNamespaces(state, 50, remoteList)
	sort.Strings(delete)

	// ns1 does not exist on the remote side, should delete
	assert.Equal(t, []string{structs.DefaultNamespace, ns1.Name}, delete)

	// ns2 is un-modified - ignore. ns3 modified, ns4 new.
	assert.Equal(t, []string{ns3.Name, ns4.Name}, update)
}
