// +build ent

// Code generated by go generate; DO NOT EDIT.
package raftutil

import "github.com/hashicorp/nomad/nomad/structs"

func init() {
	msgTypeNames[structs.NamespaceUpsertRequestType] = "NamespaceUpsertRequestType"
	msgTypeNames[structs.NamespaceDeleteRequestType] = "NamespaceDeleteRequestType"
	msgTypeNames[structs.SentinelPolicyUpsertRequestType] = "SentinelPolicyUpsertRequestType"
	msgTypeNames[structs.SentinelPolicyDeleteRequestType] = "SentinelPolicyDeleteRequestType"
	msgTypeNames[structs.QuotaSpecUpsertRequestType] = "QuotaSpecUpsertRequestType"
	msgTypeNames[structs.QuotaSpecDeleteRequestType] = "QuotaSpecDeleteRequestType"
	msgTypeNames[structs.LicenseUpsertRequestType] = "LicenseUpsertRequestType"
	msgTypeNames[structs.LicenseDeleteRequestType] = "LicenseDeleteRequestType"
	msgTypeNames[structs.TmpLicenseUpsertRequestType] = "TmpLicenseUpsertRequestType"
	msgTypeNames[structs.RecommendationUpsertRequestType] = "RecommendationUpsertRequestType"
	msgTypeNames[structs.RecommendationDeleteRequestType] = "RecommendationDeleteRequestType"
}
