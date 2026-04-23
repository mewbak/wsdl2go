package soap

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTripMIME(t *testing.T) {
	type msgT struct{ A, B string }
	type envT struct{ msgT }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/related") {
			t.Errorf("expected multipart/related, got %s", ct)
			http.Error(w, "bad content type", http.StatusBadRequest)
			return
		}

		// Parse request multipart to verify structure.
		_, params, _ := mime.ParseMediaType(ct)
		mr := multipart.NewReader(r.Body, params["boundary"])

		var parts [][]byte
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			data, _ := io.ReadAll(p)
			parts = append(parts, data)
		}

		if len(parts) < 2 {
			t.Errorf("expected at least 2 parts, got %d", len(parts))
		}

		// Echo back the SOAP envelope as a plain response.
		w.Header().Set("Content-Type", "text/xml")
		w.Write(parts[0])
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL}
	in := &msgT{A: "hello", B: "world"}
	out := &envT{}
	attachments := []Attachment{
		{ContentID: "file1", ContentType: "text/plain", Data: []byte("attachment data")},
	}

	_, err := c.RoundTripMIME("test", in, out, attachments)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out.msgT, *in) {
		t.Errorf("message mismatch\nwant: %#v\nhave: %#v", in, &out.msgT)
	}
}

func TestRoundTripMIMEMultipartResponse(t *testing.T) {
	type respT struct {
		A string
		B string
	}

	soapResp := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <A>foo</A><B>bar</B>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ct, err := mimeEncode([]byte(soapResp), "text/xml", []Attachment{
			{ContentID: "resp-file", ContentType: "application/pdf", Data: []byte("pdf-content")},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Write(body)
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL}
	out := &respT{}
	respAttachments, err := c.RoundTripMIME("test", nil, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.A != "foo" || out.B != "bar" {
		t.Errorf("unexpected response: %+v", out)
	}
	if len(respAttachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(respAttachments))
	}
	if respAttachments[0].ContentID != "resp-file" {
		t.Errorf("attachment content-id: want %q, have %q", "resp-file", respAttachments[0].ContentID)
	}
	if string(respAttachments[0].Data) != "pdf-content" {
		t.Errorf("attachment data: want %q, have %q", "pdf-content", string(respAttachments[0].Data))
	}
}

func TestRoundTripMIMEFault(t *testing.T) {
	faultXML := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <SOAP-ENV:Fault>
      <faultcode>SOAP-ENV:Server</faultcode>
      <faultstring>boom</faultstring>
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
	_, err := c.RoundTripMIME("test", nil, &out, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var f *Fault
	if !errors.As(err, &f) {
		t.Fatalf("expected *Fault, got %T: %v", err, err)
	}
}
