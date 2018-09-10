package test

import (
	"encoding/json"
	"testing"

	"github.com/jirenius/resgate/mq"
	"github.com/jirenius/resgate/reserr"
)

// Test responses to client new requests
func TestNewOnResource(t *testing.T) {

	model := resource["test.model"]
	params := json.RawMessage(`{"value":42}`)
	callResponse := json.RawMessage(`{"rid":"test.model"}`)
	modelGetResponse := json.RawMessage(`{"model":` + model + `}`)
	modelClientResponse := json.RawMessage(`{"rid":"test.model","models":{"test.model":` + resource["test.model"] + `}}`)
	modelClientInvalidParamsResponse := json.RawMessage(`{"rid":"test.model","errors":{"test.model":{"code":"system.invalidParams","message":"Invalid parameters"}}}`)
	modelClientRequestTimeoutResponse := json.RawMessage(`{"rid":"test.model","errors":{"test.model":{"code":"system.timeout","message":"Request timeout"}}}`)
	modelClientRequestAccessDeniedResponse := json.RawMessage(`{"rid":"test.model","errors":{"test.model":{"code":"system.accessDenied","message":"Access denied"}}}`)
	// Access responses
	fullCallAccess := json.RawMessage(`{"get":true,"call":"*"}`)
	methodCallAccess := json.RawMessage(`{"get":true,"call":"new"}`)
	multiMethodCallAccess := json.RawMessage(`{"get":true,"call":"foo,new"}`)
	missingMethodCallAccess := json.RawMessage(`{"get":true,"call":"foo,bar"}`)
	noCallAccess := json.RawMessage(`{"get":true}`)

	tbl := []struct {
		Params              interface{} // Params to use in call request
		CallAccessResponse  interface{} // Response on access request. requestTimeout means timeout
		CallResponse        interface{} // Response on new request. requestTimeout means timeout. noRequest means no request is expected
		GetResponse         interface{} // Response on get request of the newly created model.test. noRequest means no request is expected
		ModelAccessResponse interface{} // Response on access request of the newly created model.test.
		Expected            interface{} // Expected response to client
	}{
		// Params variants
		{params, fullCallAccess, callResponse, modelGetResponse, fullCallAccess, modelClientResponse},
		{nil, fullCallAccess, callResponse, modelGetResponse, fullCallAccess, modelClientResponse},
		// CallAccessResponse variants
		{params, methodCallAccess, callResponse, modelGetResponse, fullCallAccess, modelClientResponse},
		{params, multiMethodCallAccess, callResponse, modelGetResponse, fullCallAccess, modelClientResponse},
		{params, missingMethodCallAccess, noRequest, noRequest, noRequest, reserr.ErrAccessDenied},
		{params, noCallAccess, noRequest, noRequest, noRequest, reserr.ErrAccessDenied},
		{params, requestTimeout, noRequest, noRequest, noRequest, mq.ErrRequestTimeout},
		// CallResponse variants
		{params, fullCallAccess, reserr.ErrInvalidParams, noRequest, noRequest, reserr.ErrInvalidParams},
		{params, fullCallAccess, requestTimeout, noRequest, noRequest, mq.ErrRequestTimeout},
		// GetResponse variants
		{params, fullCallAccess, callResponse, reserr.ErrInvalidParams, fullCallAccess, modelClientInvalidParamsResponse},
		{params, fullCallAccess, callResponse, requestTimeout, fullCallAccess, modelClientRequestTimeoutResponse},
		// ModelAccessResponse variants
		{params, fullCallAccess, callResponse, modelGetResponse, json.RawMessage(`{"get":false}`), modelClientRequestAccessDeniedResponse},
		{params, fullCallAccess, callResponse, modelGetResponse, reserr.ErrInvalidParams, modelClientInvalidParamsResponse},
		{params, fullCallAccess, callResponse, modelGetResponse, reserr.ErrAccessDenied, modelClientRequestAccessDeniedResponse},
		{params, fullCallAccess, callResponse, modelGetResponse, requestTimeout, modelClientRequestTimeoutResponse},
	}

	for i, l := range tbl {
		runTest(t, func(s *Session) {
			panicked := true
			defer func() {
				if panicked {
					t.Logf("Error in test %d", i)
				}
			}()

			c := s.Connect()
			var creq *ClientRequest

			// Send client new request
			creq = c.Request("new.test.collection", l.Params)

			req := s.GetRequest(t)
			req.AssertSubject(t, "access.test.collection")
			if l.CallAccessResponse == requestTimeout {
				req.Timeout()
			} else if err, ok := l.CallAccessResponse.(*reserr.Error); ok {
				req.RespondError(err)
			} else {
				req.RespondSuccess(l.CallAccessResponse)
			}

			if l.CallResponse != noRequest {
				// Get call request
				req = s.GetRequest(t)
				req.AssertSubject(t, "call.test.collection.new")
				req.AssertPathPayload(t, "params", l.Params)
				if l.CallResponse == requestTimeout {
					req.Timeout()
				} else if err, ok := l.CallResponse.(*reserr.Error); ok {
					req.RespondError(err)
				} else {
					req.RespondSuccess(l.CallResponse)
				}
			}

			if l.GetResponse != noRequest {
				mreqs := s.GetParallelRequests(t, 2)

				var req *Request
				// Send get response
				req = mreqs.GetRequest(t, "get.test.model")
				if l.GetResponse == requestTimeout {
					req.Timeout()
				} else if err, ok := l.GetResponse.(*reserr.Error); ok {
					req.RespondError(err)
				} else {
					req.RespondSuccess(l.GetResponse)
				}

				// Send access response
				req = mreqs.GetRequest(t, "access.test.model")
				if l.ModelAccessResponse == requestTimeout {
					req.Timeout()
				} else if err, ok := l.ModelAccessResponse.(*reserr.Error); ok {
					req.RespondError(err)
				} else {
					req.RespondSuccess(l.ModelAccessResponse)
				}
			}

			// Validate client response
			cresp := creq.GetResponse(t)
			if err, ok := l.Expected.(*reserr.Error); ok {
				cresp.AssertError(t, err)
			} else if code, ok := l.Expected.(string); ok {
				cresp.AssertErrorCode(t, code)
			} else {
				cresp.AssertResult(t, l.Expected)
			}

			panicked = false
		})
	}
}