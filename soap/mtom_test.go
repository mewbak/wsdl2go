package soap

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/html/charset"
)

func TestMTOMEncodeNoExtraction(t *testing.T) {
	// Small content should not be extracted.
	envelope := `<Envelope><Body><Name>hello</Name></Body></Envelope>`
	body, ct, err := mtomEncode([]byte(envelope), "text/xml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ct, "application/xop+xml") {
		t.Errorf("expected xop content type, got %s", ct)
	}
	// Body should contain the original content (no xop:Include).
	if bytes.Contains(body, []byte("xop:Include")) {
		t.Error("unexpected xop:Include in output")
	}
	if !bytes.Contains(body, []byte("hello")) {
		t.Error("expected original content preserved")
	}
}

func TestMTOMEncodeExtraction(t *testing.T) {
	// Create a large base64 payload that exceeds the 512-byte threshold.
	rawData := bytes.Repeat([]byte("ABCD"), 256) // 1024 bytes raw
	b64 := base64.StdEncoding.EncodeToString(rawData)

	envelope := `<Envelope><Body><Data>` + b64 + `</Data></Body></Envelope>`
	body, ct, err := mtomEncode([]byte(envelope), "text/xml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ct, "application/xop+xml") {
		t.Errorf("expected xop content type, got %s", ct)
	}
	// Body should contain xop:Include reference.
	if !bytes.Contains(body, []byte("Include")) {
		t.Error("expected xop:Include in output")
	}
	// Body should contain the raw binary data as a MIME part.
	if !bytes.Contains(body, rawData) {
		t.Error("expected raw binary data in MIME part")
	}
}

func TestMTOMDecode(t *testing.T) {
	// Build a plain MTOM response (no Include references).
	xopDoc := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <Name>hello</Name>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	type respT struct {
		Name string
	}
	out := &respT{}

	body, ct, err := mtomWrap([]byte(xopDoc), "text/xml", nil)
	if err != nil {
		t.Fatal(err)
	}

	err = mtomDecode(bytes.NewReader(body), ct, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "hello" {
		t.Errorf("Name: want %q, have %q", "hello", out.Name)
	}
}

func TestMTOMDecodeWithInclude(t *testing.T) {
	rawData := []byte("the-binary-content")
	b64 := base64.StdEncoding.EncodeToString(rawData)

	// XOP document with xop:Include reference.
	xopDoc := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/"
  xmlns:xop="http://www.w3.org/2004/08/xop/include">
  <SOAP-ENV:Body>
    <Data><xop:Include href="cid:binary-part"/></Data>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	// Build MTOM multipart with the binary part.
	body, ct, err := mtomWrap([]byte(xopDoc), "text/xml", []Attachment{
		{ContentID: "binary-part", ContentType: "application/octet-stream", Data: rawData},
	})
	if err != nil {
		t.Fatal(err)
	}

	// After XOP Include resolution, the XML contains base64-encoded data.
	// Go's XML decoder through interface{} stores it as the raw string
	// in the []byte field, so we verify the base64 content.
	type respT struct {
		Data string
	}
	out := &respT{}

	err = mtomDecode(bytes.NewReader(body), ct, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Data != b64 {
		t.Errorf("data mismatch: want %q, have %q", b64, out.Data)
	}
}

func TestMTOMRoundTrip(t *testing.T) {
	type reqT struct {
		Name string
	}
	type respT struct {
		Name string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart/related") {
			t.Errorf("expected multipart/related, got %s", ct)
		}
		if !strings.Contains(ct, "application/xop+xml") {
			t.Errorf("expected application/xop+xml in content-type, got %s", ct)
		}

		// Read the request body and extract the SOAP envelope from MTOM.
		body, _ := io.ReadAll(r.Body)

		// Build a plain SOAP response (non-MTOM).
		resp := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <Name>response</Name>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`
		_ = body
		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL, MTOM: true}
	in := &reqT{Name: "test"}
	out := &respT{}
	err := c.RoundTrip(in, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "response" {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestMTOMRoundTripWithMTOMResponse(t *testing.T) {
	type respT struct {
		Name string
	}

	soapResp := `<?xml version="1.0"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">
  <SOAP-ENV:Body>
    <Name>mtom-response</Name>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return an MTOM response.
		body, ct, err := mtomWrap([]byte(soapResp), "text/xml", nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Write(body)
	}))
	defer srv.Close()

	c := &Client{URL: srv.URL, MTOM: true}
	out := &respT{}
	err := c.RoundTrip(nil, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Name != "mtom-response" {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestResolveXOPIncludes(t *testing.T) {
	rawData := []byte("binary-content-here")
	b64 := base64.StdEncoding.EncodeToString(rawData)
	xmlData := `<Root xmlns:xop="http://www.w3.org/2004/08/xop/include">
  <Field>text</Field>
  <BinaryField><xop:Include href="cid:part0@test"/></BinaryField>
</Root>`

	parts := map[string][]byte{
		"part0@test": rawData,
	}

	resolved, err := resolveXOPIncludes([]byte(xmlData), parts)
	if err != nil {
		t.Fatal(err)
	}

	// The resolved XML should contain the base64-encoded data.
	if !bytes.Contains(resolved, []byte(b64)) {
		t.Errorf("expected base64 data in resolved XML, got:\n%s", resolved)
	}

	// Verify it parses correctly. Go's XML decoder stores []byte fields
	// as raw character data, so the field contains the base64 string.
	type rootT struct {
		XMLName     xml.Name `xml:"Root"`
		Field       string
		BinaryField string
	}
	var result rootT
	decoder := xml.NewDecoder(bytes.NewReader(resolved))
	decoder.CharsetReader = charset.NewReaderLabel
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Field != "text" {
		t.Errorf("Field: want %q, have %q", "text", result.Field)
	}
	if result.BinaryField != b64 {
		t.Errorf("BinaryField: want %q, have %q", b64, result.BinaryField)
	}
}
