// +build pro ent

package nomad

import (
	"reflect"
	"testing"

	memdb "github.com/hashicorp/go-memdb"
	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/stretchr/testify/assert"
)

func TestFSM_UpsertNamespaces(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	fsm := testFSM(t)

	ns1 := mock.Namespace()
	ns2 := mock.Namespace()
	req := structs.NamespaceUpsertRequest{
		Namespaces: []*structs.Namespace{ns1, ns2},
	}
	buf, err := structs.Encode(structs.NamespaceUpsertRequestType, req)
	assert.Nil(err)
	assert.Nil(fsm.Apply(makeLog(buf)))

	// Verify we are registered
	ws := memdb.NewWatchSet()
	out, err := fsm.State().NamespaceByName(ws, ns1.Name)
	assert.Nil(err)
	assert.NotNil(out)

	out, err = fsm.State().NamespaceByName(ws, ns2.Name)
	assert.Nil(err)
	assert.NotNil(out)
}

func TestFSM_DeleteNamespaces(t *testing.T) {
	assert := assert.New(t)
	t.Parallel()
	fsm := testFSM(t)

	ns1 := mock.Namespace()
	ns2 := mock.Namespace()
	assert.Nil(fsm.State().UpsertNamespaces(1000, []*structs.Namespace{ns1, ns2}))

	req := structs.NamespaceDeleteRequest{
		Namespaces: []string{ns1.Name, ns2.Name},
	}
	buf, err := structs.Encode(structs.NamespaceDeleteRequestType, req)
	assert.Nil(err)
	assert.Nil(fsm.Apply(makeLog(buf)))

	// Verify we are NOT registered
	ws := memdb.NewWatchSet()
	out, err := fsm.State().NamespaceByName(ws, ns1.Name)
	assert.Nil(err)
	assert.Nil(out)

	out, err = fsm.State().NamespaceByName(ws, ns2.Name)
	assert.Nil(err)
	assert.Nil(out)
}

func TestFSM_SnapshotRestore_Namespaces(t *testing.T) {
	t.Parallel()
	// Add some state
	fsm := testFSM(t)
	state := fsm.State()
	ns1 := mock.Namespace()
	ns2 := mock.Namespace()
	state.UpsertNamespaces(1000, []*structs.Namespace{ns1, ns2})

	// Verify the contents
	fsm2 := testSnapshotRestore(t, fsm)
	state2 := fsm2.State()
	ws := memdb.NewWatchSet()
	out1, _ := state2.NamespaceByName(ws, ns1.Name)
	out2, _ := state2.NamespaceByName(ws, ns2.Name)
	if !reflect.DeepEqual(ns1, out1) {
		t.Fatalf("bad: \n%#v\n%#v", out1, ns1)
	}
	if !reflect.DeepEqual(ns2, out2) {
		t.Fatalf("bad: \n%#v\n%#v", out2, ns2)
	}
}
