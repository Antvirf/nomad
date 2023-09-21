// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package agent

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/nomad/nomad/structs"
)

var (
	renewLockQueryParam = "lock-renew"

	acquireLockQueryParam = string(structs.VarOpLockAcquire)
	releaseLockQueryParam = string(structs.VarOpLockRelease)
)

func (s *HTTPServer) VariablesListRequest(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	if req.Method != http.MethodGet {
		return nil, CodedError(http.StatusMethodNotAllowed, ErrInvalidMethod)
	}

	args := structs.VariablesListRequest{}
	if s.parse(resp, req, &args.Region, &args.QueryOptions) {
		return nil, CodedError(http.StatusBadRequest, "failed to parse parameters")
	}

	var out structs.VariablesListResponse
	if err := s.agent.RPC(structs.VariablesListRPCMethod, &args, &out); err != nil {
		return nil, err
	}

	setMeta(resp, &out.QueryMeta)

	if out.Data == nil {
		out.Data = make([]*structs.VariableMetadata, 0)
	}
	return out.Data, nil
}

func (s *HTTPServer) VariableSpecificRequest(resp http.ResponseWriter, req *http.Request) (interface{}, error) {
	path := strings.TrimPrefix(req.URL.Path, "/v1/var/")
	if len(path) == 0 {
		return nil, CodedError(http.StatusBadRequest, "missing variable path")
	}

	switch req.Method {
	case http.MethodGet:
		return s.variableQuery(resp, req, path)
	case http.MethodPut, http.MethodPost:
		urlParams := req.URL.Query()
		lockOperation, err := getLockOperation(urlParams)
		if err != nil {
			return nil, CodedError(http.StatusBadRequest, err.Error())
		}

		cq := req.URL.Query().Get("cas")

		if cq != "" && lockOperation != "" {
			return nil, CodedError(http.StatusBadRequest, "CAS can't be used with lock operations")
		}

		if lockOperation == "" {
			return s.variableUpsert(resp, req, path)
		}

		if lockOperation == renewLockQueryParam {
			return s.variableLockRenew(resp, req, path)
		}

		return s.variableLockOperation(resp, req, path, lockOperation)

	case http.MethodDelete:
		return s.variableDelete(resp, req, path)
	default:
		return nil, CodedError(http.StatusBadRequest, ErrInvalidMethod)
	}
}

func (s *HTTPServer) variableLockRenew(resp http.ResponseWriter, req *http.Request, path string) (interface{}, error) {

	// Parse the Variable
	var Variable structs.VariableDecrypted
	if err := decodeBody(req, &Variable); err != nil {
		return nil, CodedError(http.StatusBadRequest, err.Error())
	}

	args := structs.VariablesRenewLockRequest{
		Path:   path,
		LockID: Variable.LockID(),
	}

	s.parseWriteRequest(req, &args.WriteRequest)

	var out structs.VariablesRenewLockResponse
	if err := s.agent.RPC(structs.VariablesRenewLockRPCMethod, &args, &out); err != nil {
		return nil, err
	}

	return out.VarMeta, nil
}

func (s *HTTPServer) variableLockOperation(resp http.ResponseWriter, req *http.Request,
	path, operation string) (interface{}, error) {

	// Parse the Variable
	var Variable structs.VariableDecrypted
	if err := decodeBody(req, &Variable); err != nil {
		return nil, CodedError(http.StatusBadRequest, err.Error())
	}

	// At this point, the operation can be either acquire or release, and they are
	// both handled by the VariablesApplyRPCMethod.
	args := structs.VariablesApplyRequest{
		Op:  structs.VarOp(operation),
		Var: &Variable,
	}

	Variable.Path = path

	s.parseWriteRequest(req, &args.WriteRequest)

	var out structs.VariablesApplyResponse
	err := s.agent.RPC(structs.VariablesApplyRPCMethod, &args, &out)
	defer setIndex(resp, out.WriteMeta.Index)
	if err != nil {
		return nil, err
	}

	if out.Conflict != nil {
		resp.WriteHeader(http.StatusConflict)
		return out.Conflict, nil
	}

	// Finally, we know that this is a success response, send it to the caller
	return out.Output, nil
}

func (s *HTTPServer) variableQuery(resp http.ResponseWriter, req *http.Request,
	path string) (interface{}, error) {
	args := structs.VariablesReadRequest{
		Path: path,
	}
	if s.parse(resp, req, &args.Region, &args.QueryOptions) {
		return nil, CodedError(http.StatusBadRequest, "failed to parse parameters")
	}
	var out structs.VariablesReadResponse
	if err := s.agent.RPC(structs.VariablesReadRPCMethod, &args, &out); err != nil {
		return nil, err
	}

	setMeta(resp, &out.QueryMeta)

	if out.Data == nil {
		return nil, CodedError(http.StatusNotFound, "variable not found")
	}
	return out.Data, nil
}

func (s *HTTPServer) variableUpsert(resp http.ResponseWriter, req *http.Request,
	path string) (interface{}, error) {

	// Parse the Variable
	var Variable structs.VariableDecrypted
	if err := decodeBody(req, &Variable); err != nil {
		return nil, CodedError(http.StatusBadRequest, err.Error())
	}

	if len(Variable.Items) == 0 {
		return nil, CodedError(http.StatusBadRequest, "variable missing required Items object")
	}

	Variable.Path = path

	args := structs.VariablesApplyRequest{
		Op:  structs.VarOpSet,
		Var: &Variable,
	}

	s.parseWriteRequest(req, &args.WriteRequest)

	if isCas, checkIndex, err := parseCAS(req); err != nil {
		return nil, err
	} else if isCas {
		args.Op = structs.VarOpCAS
		args.Var.ModifyIndex = checkIndex
	}

	var out structs.VariablesApplyResponse
	if err := s.agent.RPC(structs.VariablesApplyRPCMethod, &args, &out); err != nil {

		// This handles the cases where there is an error in the CAS checking
		// function that renders it unable to return the conflicting variable
		// so it returns a text error. We can at least consider these unknown
		// moments to be CAS violations
		if strings.Contains(err.Error(), "cas error:") {
			resp.WriteHeader(http.StatusConflict)
		}

		// Otherwise it's a non-CAS error
		setIndex(resp, out.WriteMeta.Index)
		return nil, err
	}

	if out.Conflict != nil {
		setIndex(resp, out.Conflict.ModifyIndex)
		resp.WriteHeader(http.StatusConflict)
		return out.Conflict, nil
	}

	// Finally, we know that this is a success response, send it to the caller
	setIndex(resp, out.WriteMeta.Index)
	return out.Output, nil
}

func (s *HTTPServer) variableDelete(resp http.ResponseWriter, req *http.Request,
	path string) (interface{}, error) {

	args := structs.VariablesApplyRequest{
		Op: structs.VarOpDelete,
		Var: &structs.VariableDecrypted{
			VariableMetadata: structs.VariableMetadata{
				Path: path,
			},
		},
	}

	s.parseWriteRequest(req, &args.WriteRequest)

	if isCas, checkIndex, err := parseCAS(req); err != nil {
		return nil, err
	} else if isCas {
		args.Op = structs.VarOpDeleteCAS
		args.Var.ModifyIndex = checkIndex
	}

	var out structs.VariablesApplyResponse
	if err := s.agent.RPC(structs.VariablesApplyRPCMethod, &args, &out); err != nil {

		// This handles the cases where there is an error in the CAS checking
		// function that renders it unable to return the conflicting variable
		// so it returns a text error. We can at least consider these unknown
		// moments to be CAS violations
		if strings.HasPrefix(err.Error(), "cas error:") {
			resp.WriteHeader(http.StatusConflict)
		}
		setIndex(resp, out.WriteMeta.Index)
		return nil, err
	}

	// If the CAS validation can decode the conflicting value, Conflict is
	// non-Nil. Write out a 409 Conflict response.
	if out.Conflict != nil {
		setIndex(resp, out.Conflict.ModifyIndex)
		resp.WriteHeader(http.StatusConflict)
		return out.Conflict, nil
	}

	// Finally, we know that this is a success response, send it to the caller
	setIndex(resp, out.WriteMeta.Index)
	resp.WriteHeader(http.StatusNoContent)
	return nil, nil
}

func parseCAS(req *http.Request) (bool, uint64, error) {
	if cq := req.URL.Query().Get("cas"); cq != "" {
		ci, err := strconv.ParseUint(cq, 10, 64)
		if err != nil {
			return true, 0, CodedError(http.StatusBadRequest, fmt.Sprintf("can not parse cas: %v", err))
		}
		return true, ci, nil
	}
	return false, 0, nil
}

func isOneAndOnlyOneSet(a, b, c bool) bool {
	return (a || b || c) && !a != !b != !c != !(a && b && c)
}

// getLockOperation returns the lock operation to be performed in case there is
// one. It returns error if more than one is set.
func getLockOperation(queryParams url.Values) (string, error) {
	_, renewLock := queryParams[renewLockQueryParam]
	_, acquireLock := queryParams[acquireLockQueryParam]
	_, releaseLock := queryParams[releaseLockQueryParam]

	if !renewLock && !acquireLock && !releaseLock {
		return "", nil
	}

	if !isOneAndOnlyOneSet(renewLock, acquireLock, releaseLock) {
		return "", errors.New("multiple lock operations")
	}

	switch {
	case renewLock:
		return renewLockQueryParam, nil
	case acquireLock:
		return acquireLockQueryParam, nil
	case releaseLock:
		return releaseLockQueryParam, nil
	default:
		return "", errors.New("unspecified lock operation")
	}
}
