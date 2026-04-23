package soap

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

type StructFieldSetXMLData struct {
	TypeAttrXSI, TypeNamespace string
}

type SetXMLData struct {
	TypeAttrXSI, TypeNamespace string
	Pointer                    *StructFieldSetXMLData
	Field                      StructFieldSetXMLData
}

func (s *SetXMLData) SetXMLType() {
	s.TypeAttrXSI = "test"
	s.TypeNamespace = "test 1"
}

func (s *StructFieldSetXMLData) SetXMLType() {
	s.TypeAttrXSI = "struct"
	s.TypeNamespace = "struct 1"
}

func TestSetXMLType(t *testing.T) {
	type interfaceT interface{}
	type testT struct {
		A string
		B []interfaceT
	}

	test := &testT{
		A: "unchanged",
	}
	list := []*SetXMLData{{
		Pointer: &StructFieldSetXMLData{},
	}, {}}
	test.B = make([]interfaceT, len(list))
	for i, el := range list {
		test.B[i] = el
	}
	setXMLType(reflect.ValueOf(test))
	for _, interfaceEl := range test.B {
		el, _ := interfaceEl.(*SetXMLData)
		if el.TypeAttrXSI != "test" {
			t.Fatal("TypeAttrXSI not set")
		}
		if el.TypeNamespace != "test 1" {
			t.Fatal("TypeNamespace not set")
		}
		if el.Pointer != nil {
			if el.Pointer.TypeAttrXSI != "struct" {
				t.Fatal("TypeAttrXSI not set")
			}
		}
		if el.Field.TypeAttrXSI != "struct" {
			t.Fatal("TypeAttrXSI not set")
		}
	}
}

func TestRoundTrip(t *testing.T) {
	type msgT struct{ A, B string }
	type envT struct{ msgT }
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		if v := r.Header.Get("X-Test"); v != "true" {
			http.NotFound(w, r)
			return
		}
		io.Copy(w, r.Body)
	})
	s := httptest.NewServer(echo)
	defer s.Close()
	pre := func(r *http.Request) { r.Header.Set("X-Test", "true") }
	cases := []struct {
		C       *Client
		Action  string
		In, Out Message
		Fail    bool
	}{
		{
			C:      &Client{URL: s.URL, Pre: pre},
			Action: "hello",
			In:     &msgT{A: "hello", B: "world"},
			Out:    &envT{},
		},
		{
			C:      &Client{URL: s.URL, Pre: pre},
			Action: "foo",
			In:     &msgT{A: "foo", B: "bar"},
			Out:    &envT{},
		},
		{
			C:      &Client{URL: "", Pre: pre},
			Action: "none",
			Out:    &envT{},
			Fail:   true,
		},
	}
	for i, tc := range cases {
		err := tc.C.RoundTrip(tc.In, tc.Out)
		if err != nil && !tc.Fail {
			t.Errorf("test %d: %v", i, err)
			continue
		}
		if tc.Fail {
			continue
		}
		env, ok := tc.Out.(*envT)
		if !ok {
			t.Errorf("test %d: response to %#v is not an envelope", i, tc.In)
			continue
		}
		if !reflect.DeepEqual(env.msgT, *tc.In.(*msgT)) {
			t.Errorf("test %d: message mismatch\nwant: %#v\nhave: %#v",
				i, tc.In, &env.msgT)
			continue
		}
	}
}

func TestRoundTripWithAction(t *testing.T) {
	type msgT struct{ A, B string }
	type envT struct{ msgT }
	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		if v := r.Header.Get("X-Test"); v != "true" {
			http.NotFound(w, r)
			return
		}
		io.Copy(w, r.Body)
	})
	s := httptest.NewServer(echo)
	defer s.Close()
	pre := func(r *http.Request) { r.Header.Set("X-Test", "true") }
	cases := []struct {
		C       *Client
		Action  string
		In, Out Message
		Fail    bool
	}{
		{
			C:      &Client{URL: s.URL, Pre: pre},
			Action: "hello",
			In:     &msgT{A: "hello", B: "world"},
			Out:    &envT{},
		},
		{
			C:      &Client{URL: s.URL, Pre: pre},
			Action: "foo",
			In:     &msgT{A: "foo", B: "bar"},
			Out:    &envT{},
		},
		{
			C:      &Client{URL: "", Pre: pre},
			Action: "none",
			Out:    &envT{},
			Fail:   true,
		},
	}
	for i, tc := range cases {
		err := tc.C.RoundTripWithAction(tc.Action, tc.In, tc.Out)
		if err != nil && !tc.Fail {
			t.Errorf("test %d: %v", i, err)
			continue
		}
		if tc.Fail {
			continue
		}
		env, ok := tc.Out.(*envT)
		if !ok {
			t.Errorf("test %d: response to %#v is not an envelope", i, tc.In)
			continue
		}
		if !reflect.DeepEqual(env.msgT, *tc.In.(*msgT)) {
			t.Errorf("test %d: message mismatch\nwant: %#v\nhave: %#v",
				i, tc.In, &env.msgT)
			continue
		}
	}
}

func TestRoundTripSoap12(t *testing.T) {
	type msgT struct{ A, B string }
	type envT struct{ msgT }
	testAction := "http://foo.bar.com"

	echo := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		expectedContentType := fmt.Sprintf("application/soap+xml; charset=utf-8; action=\"%s\"", testAction)
		if r.Header.Get("Content-Type") != expectedContentType {
			http.NotFound(w, r)
			return
		}
		if v := r.Header.Get("X-Test"); v != "true" {
			http.NotFound(w, r)
			return
		}
		io.Copy(w, r.Body)
	})
	s := httptest.NewServer(echo)
	defer s.Close()
	pre := func(r *http.Request) { r.Header.Set("X-Test", "true") }
	cases := []struct {
		C       *Client
		Action  string
		In, Out Message
		Fail    bool
	}{
		{
			C:   &Client{URL: s.URL, Pre: pre},
			In:  &msgT{A: "hello", B: "world"},
			Out: &envT{},
		},
		{
			C:   &Client{URL: s.URL, Pre: pre},
			In:  &msgT{A: "foo", B: "bar"},
			Out: &envT{},
		},
		{
			C:    &Client{URL: "", Pre: pre},
			Out:  &envT{},
			Fail: true,
		},
	}
	for i, tc := range cases {
		err := tc.C.RoundTripSoap12(testAction, tc.In, tc.Out)
		if err != nil && !tc.Fail {
			t.Errorf("test %d: %v", i, err)
			continue
		}
		if tc.Fail {
			continue
		}
		env, ok := tc.Out.(*envT)
		if !ok {
			t.Errorf("test %d: response to %#v is not an envelope", i, tc.In)
			continue
		}
		if !reflect.DeepEqual(env.msgT, *tc.In.(*msgT)) {
			t.Errorf("test %d: message mismatch\nwant: %#v\nhave: %#v",
				i, tc.In, &env.msgT)
			continue
		}
	}
}

func TestFaultHTTP500(t *testing.T) {
	faultXML := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <SOAP-ENV:Fault>
      <faultcode>SOAP-ENV:Server</faultcode>
      <faultstring>Server error</faultstring>
      <faultactor>http://example.com/actor</faultactor>
      <detail>something broke</detail>
    </SOAP-ENV:Fault>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(faultXML))
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL}
	type respT struct{}
	var out respT
	err := c.RoundTrip(nil, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var f *Fault
	if !errors.As(err, &f) {
		t.Fatalf("expected *Fault, got %T: %v", err, err)
	}
	if f.Code != "SOAP-ENV:Server" {
		t.Errorf("fault code: want %q, have %q", "SOAP-ENV:Server", f.Code)
	}
	if f.String != "Server error" {
		t.Errorf("fault string: want %q, have %q", "Server error", f.String)
	}
	if f.Actor != "http://example.com/actor" {
		t.Errorf("fault actor: want %q, have %q", "http://example.com/actor", f.Actor)
	}
	if f.Detail != "something broke" {
		t.Errorf("fault detail: want %q, have %q", "something broke", f.Detail)
	}
}

func TestFaultHTTP200(t *testing.T) {
	faultXML := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <SOAP-ENV:Fault>
      <faultcode>SOAP-ENV:Client</faultcode>
      <faultstring>Invalid request</faultstring>
    </SOAP-ENV:Fault>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(faultXML))
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL}
	type respT struct{}
	var out respT
	err := c.RoundTrip(nil, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var f *Fault
	if !errors.As(err, &f) {
		t.Fatalf("expected *Fault, got %T: %v", err, err)
	}
	if f.Code != "SOAP-ENV:Client" {
		t.Errorf("fault code: want %q, have %q", "SOAP-ENV:Client", f.Code)
	}
}

func TestHTTPErrorNonFault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL}
	type respT struct{}
	var out respT
	err := c.RoundTrip(nil, &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusBadGateway {
		t.Errorf("status: want %d, have %d", http.StatusBadGateway, httpErr.StatusCode)
	}
}
