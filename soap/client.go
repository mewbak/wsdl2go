// Package soap provides a SOAP HTTP client.
package soap

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"golang.org/x/net/html/charset"
)

// Fault represents a SOAP fault returned by the server.
type Fault struct {
	Code   string `xml:"faultcode"`
	String string `xml:"faultstring"`
	Actor  string `xml:"faultactor,omitempty"`
	Detail string `xml:"detail,omitempty"`
}

func (f *Fault) Error() string {
	return fmt.Sprintf("soap fault: %s: %s", f.Code, f.String)
}

// XSINamespace is a link to the XML Schema instance namespace.
const XSINamespace = "http://www.w3.org/2001/XMLSchema-instance"

var xmlTyperType reflect.Type = reflect.TypeOf((*XMLTyper)(nil)).Elem()

// A RoundTripper executes a request passing the given req as the SOAP
// envelope body. The HTTP response is then de-serialized onto the resp
// object. Returns error in case an error occurs serializing req, making
// the HTTP request, or de-serializing the response.
type RoundTripper interface {
	RoundTrip(req, resp Message) error
	RoundTripSoap12(action string, req, resp Message) error
}

// Message is an opaque type used by the RoundTripper to carry XML
// documents for SOAP.
type Message interface{}

// Header is an opaque type used as the SOAP Header element in requests.
type Header interface{}

// AuthHeader is a Header to be encoded as the SOAP Header element in
// requests, to convey credentials for authentication.
type AuthHeader struct {
	Namespace string `xml:"xmlns:ns,attr"`
	Username  string `xml:"ns:username"`
	Password  string `xml:"ns:password"`
}

// Client is a SOAP client.
type Client struct {
	URL                    string               // URL of the server
	UserAgent              string               // User-Agent header will be added to each request
	Namespace              string               // SOAP Namespace
	URNamespace            string               // Uniform Resource Namespace
	ThisNamespace          string               // SOAP This-Namespace (tns)
	ExcludeActionNamespace bool                 // Include Namespace to SOAP Action header
	Envelope               string               // Optional SOAP Envelope
	Header                 Header               // Optional SOAP Header
	ContentType            string               // Optional Content-Type (default text/xml)
	Config                 *http.Client         // Optional HTTP client
	Pre                    func(*http.Request)  // Optional hook to modify outbound requests
	Post                   func(*http.Response) // Optional hook to snoop inbound responses
	Ctx                    context.Context      // Optional variable to allow Context Tracking.
	MTOM                   bool                 // Use MTOM/XOP for binary content
}

// XMLTyper is an abstract interface for types that can set an XML type.
type XMLTyper interface {
	SetXMLType()
}

func setXMLType(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	switch v.Type().Kind() {
	case reflect.Interface:
		setXMLType(v.Elem())
	case reflect.Ptr:
		if v.IsNil() {
			break
		}
		ok := v.Type().Implements(xmlTyperType)
		if ok {
			v.MethodByName("SetXMLType").Call(nil)
		}
		setXMLType(v.Elem())
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			setXMLType(v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanAddr() {
				setXMLType(v.Field(i).Addr())
			} else {
				setXMLType(v.Field(i))
			}
		}
	}
}

func doRoundTrip(ctx context.Context, c *Client, setHeaders func(*http.Request), in, out Message) error {
	setXMLType(reflect.ValueOf(in))
	req := &Envelope{
		EnvelopeAttr: c.Envelope,
		URNAttr:      c.URNamespace,
		NSAttr:       c.Namespace,
		TNSAttr:      c.ThisNamespace,
		XSIAttr:      XSINamespace,
		Header:       c.Header,
		Body:         in,
	}

	if req.EnvelopeAttr == "" {
		req.EnvelopeAttr = "http://schemas.xmlsoap.org/soap/envelope/"
	}
	if req.NSAttr == "" {
		req.NSAttr = c.URL
	}
	if req.TNSAttr == "" {
		req.TNSAttr = req.NSAttr
	}
	var envBuf bytes.Buffer
	err := xml.NewEncoder(&envBuf).Encode(req)
	if err != nil {
		return err
	}

	cli := c.Config
	if cli == nil {
		cli = http.DefaultClient
	}

	var reqBody io.Reader
	if c.MTOM {
		ct := c.ContentType
		if ct == "" {
			ct = "text/xml"
		}
		mtomBody, mtomCT, err := mtomEncode(envBuf.Bytes(), ct)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(mtomBody)
		// Override setHeaders to use the MTOM content type.
		origSetHeaders := setHeaders
		setHeaders = func(r *http.Request) {
			origSetHeaders(r)
			r.Header.Set("Content-Type", mtomCT)
		}
	} else {
		reqBody = &envBuf
	}

	r, err := http.NewRequest("POST", c.URL, reqBody)
	if err != nil {
		return err
	}
	setHeaders(r)
	if c.Pre != nil {
		c.Pre(r)
	}

	r = r.WithContext(ctx)

	resp, err := cli.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if c.Post != nil {
		c.Post(resp)
	}
	// Buffer the response body so we can inspect it for faults.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return err
	}

	// Check for SOAP fault: HTTP 500 is the standard fault status,
	// but some servers return faults with HTTP 200.
	if resp.StatusCode == http.StatusInternalServerError || bytes.Contains(body, []byte("Fault>")) {
		var faultEnv struct {
			XMLName xml.Name `xml:"Envelope"`
			Body    struct {
				Fault *Fault `xml:"Fault"`
			} `xml:"Body"`
		}
		decoder := xml.NewDecoder(bytes.NewReader(body))
		decoder.CharsetReader = charset.NewReaderLabel
		if err := decoder.Decode(&faultEnv); err == nil && faultEnv.Body.Fault != nil {
			return faultEnv.Body.Fault
		}
	}

	if resp.StatusCode != http.StatusOK {
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Msg:        string(body),
		}
	}

	// Check if response is MTOM.
	respCT := resp.Header.Get("Content-Type")
	if strings.Contains(respCT, "application/xop+xml") || strings.Contains(respCT, "multipart/related") {
		return mtomDecode(bytes.NewReader(body), respCT, out)
	}

	marshalStructure := struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    Message
	}{Body: out}

	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.CharsetReader = charset.NewReaderLabel
	return decoder.Decode(&marshalStructure)
}

func (c *Client) ctx() context.Context {
	if c.Ctx != nil {
		return c.Ctx
	}
	return context.Background()
}

// RoundTrip implements the RoundTripper interface.
func (c *Client) RoundTrip(in, out Message) error {
	return c.RoundTripContext(c.ctx(), in, out)
}

// RoundTripContext is like RoundTrip but with an explicit context.
func (c *Client) RoundTripContext(ctx context.Context, in, out Message) error {
	headerFunc := func(r *http.Request) {
		if c.UserAgent != "" {
			r.Header.Add("User-Agent", c.UserAgent)
		}
		var actionName, soapAction string
		if in != nil {
			soapAction = reflect.TypeOf(in).Elem().Name()
		}
		ct := c.ContentType
		if ct == "" {
			ct = "text/xml"
		}
		r.Header.Set("Content-Type", ct)
		if in != nil {
			if c.ExcludeActionNamespace {
				actionName = soapAction
			} else {
				actionName = fmt.Sprintf("%s/%s", c.Namespace, soapAction)
			}
			r.Header.Add("SOAPAction", fmt.Sprintf("%q", actionName))
		}
	}
	return doRoundTrip(ctx, c, headerFunc, in, out)
}

// RoundTripWithAction implements the RoundTripper interface for SOAP clients
// that need to set the SOAPAction header.
func (c *Client) RoundTripWithAction(soapAction string, in, out Message) error {
	return c.RoundTripWithActionContext(c.ctx(), soapAction, in, out)
}

// RoundTripWithActionContext is like RoundTripWithAction but with an explicit context.
func (c *Client) RoundTripWithActionContext(ctx context.Context, soapAction string, in, out Message) error {
	headerFunc := func(r *http.Request) {
		if c.UserAgent != "" {
			r.Header.Add("User-Agent", c.UserAgent)
		}
		var actionName string
		ct := c.ContentType
		if ct == "" {
			ct = "text/xml"
		}
		r.Header.Set("Content-Type", ct)
		if in != nil {
			if c.ExcludeActionNamespace {
				actionName = soapAction
			} else {
				actionName = fmt.Sprintf("%s/%s", c.Namespace, soapAction)
			}
			r.Header.Add("SOAPAction", fmt.Sprintf("%q", actionName))
		}
	}
	return doRoundTrip(ctx, c, headerFunc, in, out)
}

// RoundTripSoap12 implements the RoundTripper interface for SOAP 1.2.
func (c *Client) RoundTripSoap12(action string, in, out Message) error {
	return c.RoundTripSoap12Context(c.ctx(), action, in, out)
}

// RoundTripSoap12Context is like RoundTripSoap12 but with an explicit context.
func (c *Client) RoundTripSoap12Context(ctx context.Context, action string, in, out Message) error {
	headerFunc := func(r *http.Request) {
		r.Header.Add("Content-Type", fmt.Sprintf("application/soap+xml; charset=utf-8; action=\"%s\"", action))
	}
	return doRoundTrip(ctx, c, headerFunc, in, out)
}

// RoundTripMIME sends a SOAP request with MIME attachments and returns
// any attachments from the response.
func (c *Client) RoundTripMIME(soapAction string, in, out Message, attachments []Attachment) ([]Attachment, error) {
	setXMLType(reflect.ValueOf(in))
	req := &Envelope{
		EnvelopeAttr: c.Envelope,
		URNAttr:      c.URNamespace,
		NSAttr:       c.Namespace,
		TNSAttr:      c.ThisNamespace,
		XSIAttr:      XSINamespace,
		Header:       c.Header,
		Body:         in,
	}
	if req.EnvelopeAttr == "" {
		req.EnvelopeAttr = "http://schemas.xmlsoap.org/soap/envelope/"
	}
	if req.NSAttr == "" {
		req.NSAttr = c.URL
	}
	if req.TNSAttr == "" {
		req.TNSAttr = req.NSAttr
	}
	var envBuf bytes.Buffer
	if err := xml.NewEncoder(&envBuf).Encode(req); err != nil {
		return nil, err
	}

	ct := c.ContentType
	if ct == "" {
		ct = "text/xml"
	}
	mimeBody, mimeContentType, err := mimeEncode(envBuf.Bytes(), ct, attachments)
	if err != nil {
		return nil, err
	}

	cli := c.Config
	if cli == nil {
		cli = http.DefaultClient
	}
	r, err := http.NewRequest("POST", c.URL, bytes.NewReader(mimeBody))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", mimeContentType)
	if soapAction != "" {
		var actionName string
		if c.ExcludeActionNamespace {
			actionName = soapAction
		} else {
			actionName = fmt.Sprintf("%s/%s", c.Namespace, soapAction)
		}
		r.Header.Add("SOAPAction", fmt.Sprintf("%q", actionName))
	}
	if c.UserAgent != "" {
		r.Header.Add("User-Agent", c.UserAgent)
	}
	if c.Pre != nil {
		c.Pre(r)
	}
	r = r.WithContext(c.ctx())

	resp, err := cli.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if c.Post != nil {
		c.Post(resp)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, err
	}

	// Check for SOAP fault.
	if resp.StatusCode == http.StatusInternalServerError || bytes.Contains(body, []byte("Fault>")) {
		var faultEnv struct {
			XMLName xml.Name `xml:"Envelope"`
			Body    struct {
				Fault *Fault `xml:"Fault"`
			} `xml:"Body"`
		}
		decoder := xml.NewDecoder(bytes.NewReader(body))
		decoder.CharsetReader = charset.NewReaderLabel
		if err := decoder.Decode(&faultEnv); err == nil && faultEnv.Body.Fault != nil {
			return nil, faultEnv.Body.Fault
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Msg:        string(body),
		}
	}

	// Check if response is multipart.
	respCT := resp.Header.Get("Content-Type")
	if strings.HasPrefix(respCT, "multipart/related") {
		return mimeDecode(bytes.NewReader(body), respCT, out)
	}

	// Plain SOAP response.
	marshalStructure := struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    Message
	}{Body: out}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.CharsetReader = charset.NewReaderLabel
	return nil, decoder.Decode(&marshalStructure)
}

// HTTPError is detailed soap http error
type HTTPError struct {
	StatusCode int
	Status     string
	Msg        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%q: %q", e.Status, e.Msg)
}

// Envelope is a SOAP envelope.
type Envelope struct {
	XMLName      xml.Name `xml:"SOAP-ENV:Envelope"`
	EnvelopeAttr string   `xml:"xmlns:SOAP-ENV,attr"`
	NSAttr       string   `xml:"xmlns:ns,attr"`
	TNSAttr      string   `xml:"xmlns:tns,attr,omitempty"`
	URNAttr      string   `xml:"xmlns:urn,attr,omitempty"`
	XSIAttr      string   `xml:"xmlns:xsi,attr,omitempty"`
	Header       Message  `xml:"SOAP-ENV:Header"`
	Body         Message  `xml:"SOAP-ENV:Body"`
}
