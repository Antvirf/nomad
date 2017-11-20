// +build pro ent

package structs

import (
	"fmt"
	"regexp"

	"golang.org/x/crypto/blake2b"

	multierror "github.com/hashicorp/go-multierror"
)

// Offset the Nomad Pro specific values so that we don't overlap
// the OSS/Enterprise values.
const (
	NamespaceUpsertRequestType MessageType = (64 + iota)
	NamespaceDeleteRequestType
	SentinelPolicyUpsertRequestType
	SentinelPolicyDeleteRequestType
	QuotaSpecUpsertRequestType
	QuotaSpecDeleteRequestType
)

var (
	// validNamespaceName is used to validate a namespace name
	validNamespaceName = regexp.MustCompile("^[a-zA-Z0-9-]{1,128}$")
)

const (
	// maxNamespaceDescriptionLength limits a namespace description length
	maxNamespaceDescriptionLength = 256
)

// Namespace allows logically grouping jobs and their associated objects.
type Namespace struct {
	// Name is the name of the namespace
	Name string

	// Description is a human readable description of the namespace
	Description string

	// Quota is the quota specification that the namespace should account
	// against.
	Quota string

	// Hash is the hash of the namespace which is used to efficiently replicate
	// cross-regions.
	Hash []byte

	// Raft Indexes
	CreateIndex uint64
	ModifyIndex uint64
}

func (n *Namespace) Validate() error {
	var mErr multierror.Error

	// Validate the name and description
	if !validNamespaceName.MatchString(n.Name) {
		err := fmt.Errorf("invalid name %q. Must match regex %s", n.Name, validNamespaceName)
		mErr.Errors = append(mErr.Errors, err)
	}
	if len(n.Description) > maxNamespaceDescriptionLength {
		err := fmt.Errorf("description longer than %d", maxNamespaceDescriptionLength)
		mErr.Errors = append(mErr.Errors, err)
	}

	return mErr.ErrorOrNil()
}

// SetHash is used to compute and set the hash of the namespace
func (n *Namespace) SetHash() []byte {
	// Initialize a 256bit Blake2 hash (32 bytes)
	hash, err := blake2b.New256(nil)
	if err != nil {
		panic(err)
	}

	// Write all the user set fields
	hash.Write([]byte(n.Name))
	hash.Write([]byte(n.Description))
	hash.Write([]byte(n.Quota))

	// Finalize the hash
	hashVal := hash.Sum(nil)

	// Set and return the hash
	n.Hash = hashVal
	return hashVal
}

func (n *Namespace) Copy() *Namespace {
	nc := new(Namespace)
	*nc = *n
	nc.Hash = make([]byte, len(n.Hash))
	copy(nc.Hash, n.Hash)
	return nc
}

// NamespaceListRequest is used to request a list of namespaces
type NamespaceListRequest struct {
	QueryOptions
}

// NamespaceListResponse is used for a list request
type NamespaceListResponse struct {
	Namespaces []*Namespace
	QueryMeta
}

// NamespaceSpecificRequest is used to query a specific namespace
type NamespaceSpecificRequest struct {
	Name string
	QueryOptions
}

// SingleNamespaceResponse is used to return a single namespace
type SingleNamespaceResponse struct {
	Namespace *Namespace
	QueryMeta
}

// NamespaceSetRequest is used to query a set of namespaces
type NamespaceSetRequest struct {
	Namespaces []string
	QueryOptions
}

// NamespaceSetResponse is used to return a set of namespaces
type NamespaceSetResponse struct {
	Namespaces map[string]*Namespace // Keyed by namespace Name
	QueryMeta
}

// NamespaceDeleteRequest is used to delete a set of namespaces
type NamespaceDeleteRequest struct {
	Namespaces []string
	WriteRequest
}

// NamespaceUpsertRequest is used to upsert a set of namespaces
type NamespaceUpsertRequest struct {
	Namespaces []*Namespace
	WriteRequest
}
