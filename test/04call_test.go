package test

import (
	"encoding/json"
	"testing"

	"github.com/jirenius/resgate/mq"
	"github.com/jirenius/resgate/reserr"
)

// Test responses to client call requests
func TestCallOnResource(t *testing.T) {

	model := resource["test.model"]
	params := json.RawMessage(`{"value":42}`)
	successResponse := json.RawMessage(`{"foo":"bar"}`)
	// Access responses
	fullCallAccess := json.RawMessage(`{"get":true,"call":"*"}`)
	methodCallAccess := json.RawMessage(`{"get":true,"call":"method"}`)
	multiMethodCallAccess := json.RawMessage(`{"get":true,"call":"foo,method"}`)
	missingMethodCallAccess := json.RawMessage(`{"get":true,"call":"foo,bar"}`)
	noCallAccess := json.RawMessage(`{"get":true}`)

	tbl := []struct {
		Subscribe      bool        // Flag if model is subscribed prior to call
		Params         interface{} // Params to use in call request
		AccessResponse interface{} // Response on access request. nil means timeout
		CallResponse   interface{} // Response on call request. requestTimeout means timeout. noRequest means no call request is expected
		Expected       interface{}
	}{
		{true, nil, fullCallAccess, successResponse, successResponse},
		{false, nil, fullCallAccess, successResponse, successResponse},
		{true, nil, methodCallAccess, successResponse, successResponse},
		{false, nil, methodCallAccess, successResponse, successResponse},
		{true, nil, multiMethodCallAccess, successResponse, successResponse},
		{false, nil, multiMethodCallAccess, successResponse, successResponse},
		{false, nil, missingMethodCallAccess, noRequest, reserr.ErrAccessDenied},
		{false, nil, noCallAccess, noRequest, reserr.ErrAccessDenied},
		{false, nil, nil, noRequest, mq.ErrRequestTimeout},
		{true, nil, fullCallAccess, reserr.ErrInvalidParams, reserr.ErrInvalidParams},
		{false, nil, fullCallAccess, reserr.ErrInvalidParams, reserr.ErrInvalidParams},
		{true, params, fullCallAccess, successResponse, successResponse},
		{false, params, fullCallAccess, successResponse, successResponse},
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

			if l.Subscribe {
				creq = c.Request("subscribe.test.model", nil)

				// Handle model get and access request
				mreqs := s.GetParallelRequests(t, 2)
				req := mreqs.GetRequest(t, "access.test.model")
				if l.AccessResponse == nil {
					req.Timeout()
				} else if err, ok := l.AccessResponse.(*reserr.Error); ok {
					req.RespondError(err)
				} else {
					req.RespondSuccess(l.AccessResponse)
				}
				req = mreqs.GetRequest(t, "get.test.model")
				req.RespondSuccess(json.RawMessage(`{"model":` + model + `}`))

				// Get client response
				creq.GetResponse(t)

				// Send client call request
				creq = c.Request("call.test.model.method", l.Params)
				if l.CallResponse != noRequest {
					// Get call request
					req = s.GetRequest(t)
					req.AssertSubject(t, "call.test.model.method")
					req.AssertPathType(t, "cid", string(""))
					req.AssertPathPayload(t, "token", nil)
					req.AssertPathPayload(t, "params", l.Params)
					if l.CallResponse == requestTimeout {
						req.Timeout()
					} else if err, ok := l.CallResponse.(*reserr.Error); ok {
						req.RespondError(err)
					} else {
						req.RespondSuccess(l.CallResponse)
					}
				}
			} else {
				// Send client call request
				creq = c.Request("call.test.model.method", l.Params)

				req := s.GetRequest(t)
				req.AssertSubject(t, "access.test.model")
				if l.AccessResponse == nil {
					req.Timeout()
				} else if err, ok := l.AccessResponse.(*reserr.Error); ok {
					req.RespondError(err)
				} else {
					req.RespondSuccess(l.AccessResponse)
				}

				if l.CallResponse != noRequest {
					// Get call request
					req = s.GetRequest(t)
					req.AssertSubject(t, "call.test.model.method")
					req.AssertPathPayload(t, "params", l.Params)
					if l.CallResponse == requestTimeout {
						req.Timeout()
					} else if err, ok := l.CallResponse.(*reserr.Error); ok {
						req.RespondError(err)
					} else {
						req.RespondSuccess(l.CallResponse)
					}
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