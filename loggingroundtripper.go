package loggingroundtripper

import "net/http"
import "sync/atomic"
import "time"
import "log"
import "io/ioutil"
import "strings"
import "bytes"
import "github.com/drewlesueur/http2curl"

// newHTTPClient is a helper that gets us an http client with a custom roundtripper
// that will log every request and response.
// It will also optionally not make requests, just log them.
// This will be helpful for viewing what we would send and not sending it.
type LoggingRoundTripper struct {
	LogOnly        bool
	InnerTransport http.RoundTripper
}

var requestIDCounter uint64

func (l *LoggingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	// FUTURE: you can additionally have a higher level tracing id that comes in with the r.Context.Value("id") or something like that.

	requestID := atomic.AddUint64(&requestIDCounter, 1)
	curlCommand, err := http2curl.GetCurlCommand(r)
	if err != nil {
		log.Printf("loggingroundtripper|ERROR||req:%d|LoggingRoundTripper: converting request to curl: %v", requestID, err)
		// Not returning early here, because if this logging fails, we still want to go ahead with the request.
	}

	if l.LogOnly {
		// if we are configured to only log, then let's respond with a dummy response
		dummyResp := &http.Response{
			Status:           "200 OK",
			StatusCode:       200,
			Proto:            "HTTP/1.0",
			ProtoMajor:       1,
			ProtoMinor:       0,
			Header:           map[string][]string{},
			Body:             ioutil.NopCloser(strings.NewReader("")),
			ContentLength:    -1,
			TransferEncoding: nil,
			Uncompressed:     false,
			Trailer:          map[string][]string{},
			Request:          r,
			TLS:              nil,
		}
		log.Printf("loggingroundtripper|INFO||req:%d|Pretend HTTP Request: %s", requestID, curlCommand)
		log.Printf("loggingroundtripper|INFO||req:%d|Pretend HTTP Response: URL:%s, StatusCode:%d, Header:%+v, Body:%s", requestID, r.URL.String(), dummyResp.StatusCode, dummyResp.Header, "")
		return dummyResp, nil
	} else {
		log.Printf("loggingroundtripper|INFO||req:%d|Real HTTP Request: %s", requestID, curlCommand)
	}

	start := time.Now()

	var transport = l.InnerTransport
	if transport == nil {
		transport = http.DefaultTransport
	}

	resp, err := transport.RoundTrip(r)
	if err != nil {
		log.Printf("loggingroundtripper|ERROR||req:%d|request: %v", requestID, err)
		return resp, err
	}
	defer resp.Body.Close()

	// Eat the response, but generate a new one.
	// Note that this reads the whole body into memory.
	// For the common use case I do that anyway, but for streaming/copying super large datasets this will not work.
	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		// Returning the err for ReadAll, when the calling code thinks it's an error to RoundTrip().
		// Still, the behavior should be the same. If calling code gets an errror, it should not try to read the body.
		log.Printf("loggingroundtripper|ERROR||req:%d|reading body: %v", requestID, err)
		return resp, err
	}

	// FUTURE: Consider stripping newlines
	log.Printf("loggingroundtripper|INFO||req:%d|Real HTTP Response: Duration:%s, URL:%s, StatusCode:%d, Header:%+v, Body:%s", requestID, time.Since(start), r.URL.String(), resp.StatusCode, resp.Header, string(respBody))
	// Re-set the body as if we never read from it.
	resp.Body = ioutil.NopCloser(bytes.NewReader(respBody))
	return resp, nil
}