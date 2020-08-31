// +build ent

package raftutil

import "github.com/hashicorp/nomad/nomad/state"

func insertEnterpriseState(m map[string][]interface{}, state *state.StateStore) {
	m["Namespaces"] = toArray(state.Namespaces(nil))
	m["SentinelPolicies"] = toArray(state.SentinelPolicies(nil))
	m["QuotaSpecs"] = toArray(state.QuotaSpecs(nil))
	m["QuotaUsages"] = toArray(state.QuotaUsages(nil))
}
