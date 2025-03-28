package service

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/rockbears/log"
	"github.com/rockbears/yaml"
	"gopkg.in/spacemonkeygo/httpsig.v0"

	"github.com/ovh/cds/sdk"
	"github.com/ovh/cds/sdk/cdsclient"
	cdslog "github.com/ovh/cds/sdk/log"
)

// Handler defines the HTTP handler used in CDS engine
type Handler func(ctx context.Context, w http.ResponseWriter, r *http.Request) error
type RbacChecker func(ctx context.Context, vars map[string]string) error
type RbacCheckers []RbacChecker

func RBAC(checkers ...RbacChecker) []RbacChecker {
	return checkers
}

// AsynchronousHandler defines the HTTP asynchronous handler used in CDS engine
type AsynchronousHandler func(ctx context.Context, r *http.Request) error

// Middleware defines the HTTP Middleware used in CDS engine
type Middleware func(ctx context.Context, w http.ResponseWriter, req *http.Request, rc *HandlerConfig) (context.Context, error)

// HandlerFunc defines the way to instantiate a handler
type HandlerFunc func() Handler
type HandlerFuncV2 func() ([]RbacChecker, Handler)

// AsynchronousHandlerFunc defines the way to instantiate a handler
type AsynchronousHandlerFunc func() AsynchronousHandler

// RouterConfigParam is the type of anonymous function returned by POST, GET and PUT functions
type RouterConfigParam func(rc *RouterConfig)

// RouterConfig contains a map of handler configuration. Key is the method of the http route
type RouterConfig struct {
	Config map[string]*HandlerConfig
}

// HandlerConfig is the configuration for one handler
type HandlerConfig struct {
	Name                   string
	Method                 string
	Handler                Handler
	IsDeprecated           bool
	OverrideAuthMiddleware Middleware
	MaintenanceAware       bool
	AllowedScopes          []sdk.AuthConsumerScope
	PermissionLevel        int
	CleanURL               string
	RbacCheckers           []RbacChecker
}

// Accepted is a helper function used by asynchronous handlers
func Accepted(w http.ResponseWriter) error {
	const msg = "request accepted"
	w.Header().Add("Content-Type", "text/plain")
	w.Header().Add("Content-Length", fmt.Sprintf("%d", len(msg)))
	w.WriteHeader(http.StatusAccepted)
	_, err := w.Write([]byte(msg))
	return err
}

// Write is a helper function
func Write(w http.ResponseWriter, r io.Reader, status int, contentType string) error {
	w.Header().Add("Content-Type", contentType)

	WriteProcessTime(context.TODO(), w)
	w.WriteHeader(status)

	n, err := io.Copy(w, r)
	if err != nil {
		return sdk.WithStack(err)
	}

	w.Header().Add("Content-Length", fmt.Sprintf("%d", n))

	return nil
}

// map to avoid to register twice the same action_metadata field
var registeredActionMetadataFiels = new(sync.Map)

func TrackActionMetadataFromFields(w http.ResponseWriter, data interface{}) {
	if responseTracker := UnwrapResponseWriter(w); responseTracker != nil && data != nil {
		dataV := sdk.ValueFromInterface(data)
		dataT := dataV.Type()
		if dataT.Kind() == reflect.Struct {
			for i := 0; i < dataT.NumField(); i++ {
				v := dataV.Field(i)
				t, hasTag := dataT.Field(i).Tag.Lookup("action_metadata")
				if v.Kind() == reflect.Struct && hasTag {
					TrackActionMetadataFromFields(w, v.Interface())
				} else {
					if hasTag && fmt.Sprintf("%v", v.Interface()) != "" {
						var f = log.Field("action_metadata_" + t)
						if _, exist := registeredActionMetadataFiels.Load(f); !exist {
							registeredActionMetadataFiels.Store(f, struct{}{})
							log.RegisterField(f)
						}
						SetTracker(responseTracker, f, v.Interface())
					}
				}
			}
		}
	}
}

// WriteJSON is a helper function to marshal json, handle errors and set Content-Type for the best
func WriteJSON(w http.ResponseWriter, data interface{}, status int) error {
	TrackActionMetadataFromFields(w, data)
	b, err := json.Marshal(data)
	if err != nil {
		return sdk.WrapError(err, "Unable to marshal json data")
	}
	return sdk.WithStack(Write(w, bytes.NewReader(b), status, "application/json"))
}

// WriteMarshal is a helper function to marshal json/yaml, handle errors and set Content-Type for the best
// Response format could be application/json or appliation/x-yaml, depends on the Accept header
// default response is application/x-yaml
func WriteMarshal(w http.ResponseWriter, req *http.Request, data interface{}, status int) error {
	TrackActionMetadataFromFields(w, data)

	var contentType string
	var body []byte
	var err error

	if req.Header.Get("Accept") == "application/json" {
		contentType = "application/json"
		body, err = yaml.Marshal(data)
	} else { // yaml is the default response
		contentType = "application/x-yaml"
		body, err = json.Marshal(data)
	}

	if err != nil {
		return sdk.WrapError(err, "unable to marshal data into %s", contentType)
	}

	return sdk.WithStack(Write(w, bytes.NewReader(body), status, contentType))
}

// WriteProcessTime writes the duration of the call in the responsewriter
func WriteProcessTime(ctx context.Context, w http.ResponseWriter) {
	if h := w.Header().Get(cdsclient.ResponseAPINanosecondsTimeHeader); h != "" {
		start, err := strconv.ParseInt(h, 10, 64)
		if err != nil {
			log.Error(ctx, "WriteProcessTime> error on ParseInt header ResponseAPINanosecondsTimeHeader: %s", err)
		}
		w.Header().Add(cdsclient.ResponseProcessTimeHeader, fmt.Sprintf("%d", time.Now().UnixNano()-start))
	}
}

type ErrorResponse struct {
	sdk.Error
	RequestID string `json:"request_id"`
}

// WriteError is a helper function to return error in a language the called understand
func WriteError(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) {
	httpErr := sdk.ExtractHTTPError(err)

	requestID := cdslog.ContextValue(ctx, cdslog.RequestID)
	httpErr.RequestID = requestID

	ctx = sdk.ContextWithStacktrace(ctx, err)

	if httpErr.Status < 500 {
		log.Info(ctx, "%s", err)
	} else {
		log.ErrorWithStackTrace(ctx, err)
	}

	// safely ignore error returned by WriteJSON
	_ = WriteJSON(w, httpErr, httpErr.Status)
}

// UnmarshalBody read the request body and tries to json.unmarshal it. It returns sdk.ErrWrongRequest in case of error.
func UnmarshalBody(r *http.Request, i interface{}) error {
	if r == nil {
		return sdk.NewErrorFrom(sdk.ErrWrongRequest, "request is null")
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return sdk.NewError(sdk.ErrWrongRequest, err)
	}
	defer r.Body.Close()
	if len(data) == 0 {
		return nil
	}
	if err := sdk.JSONUnmarshal(data, i); err != nil {
		return sdk.NewError(sdk.ErrWrongRequest, err)
	}
	return nil
}

// UnmarshalRequest unmarshal the request into the specified entity.
// The body request can be a JSON or a YAML format
func UnmarshalRequest(ctx context.Context, req *http.Request, entity interface{}) error {
	if req == nil {
		return sdk.NewErrorFrom(sdk.ErrWrongRequest, "request is null")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return sdk.NewError(sdk.ErrWrongRequest, err)
	}
	defer req.Body.Close()

	if err := yaml.Unmarshal(body, entity); err != nil {
		return err
	}

	return nil
}

type httpVerifier struct {
	sync.Mutex
	pubKey *rsa.PublicKey
}

func (v *httpVerifier) SetKey(pubKey *rsa.PublicKey) {
	v.Lock()
	defer v.Unlock()
	v.pubKey = pubKey
}

func (v *httpVerifier) GetKey(id string) interface{} {
	v.Lock()
	defer v.Unlock()
	return v.pubKey
}

var (
	_                  httpsig.KeyGetter = new(httpVerifier)
	globalHTTPVerifier *httpVerifier
)

func CheckRequestSignatureMiddleware(pubKey *rsa.PublicKey) Middleware {
	globalHTTPVerifier = new(httpVerifier)
	globalHTTPVerifier.SetKey(pubKey)

	verifier := httpsig.NewVerifier(globalHTTPVerifier)
	verifier.SetRequiredHeaders([]string{"(request-target)", "host", "date"})

	return func(ctx context.Context, w http.ResponseWriter, req *http.Request, rc *HandlerConfig) (context.Context, error) {
		if err := verifier.Verify(req); err != nil {
			return ctx, sdk.NewError(sdk.ErrUnauthorized, err)
		}
		return ctx, nil
	}
}

// FormInt64 return a int64.
func FormInt64(r *http.Request, s string) int64 {
	i, _ := strconv.ParseInt(r.FormValue(s), 10, 64)
	return i
}

// FormInt return a int.
func FormInt(r *http.Request, s string) int {
	i, _ := strconv.Atoi(r.FormValue(s))
	return i
}

// FormUInt return a uint.
func FormUInt(r *http.Request, s string) uint {
	i := FormInt(r, s)
	if i < 0 {
		return 0
	}
	return uint(i)
}

// FormBool return true if the form value is set to true|TRUE|yes|YES|1
func FormBool(r *http.Request, s string) bool {
	v := r.FormValue(s)
	switch v {
	case "true", "TRUE", "yes", "YES", "1":
		return true
	default:
		return false
	}
}
