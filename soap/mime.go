package soap

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"

	"golang.org/x/net/html/charset"
)

// Attachment represents a MIME attachment in a SOAP message.
type Attachment struct {
	ContentID   string
	ContentType string
	Data        []byte
}

// mimeEncode creates a multipart/related message with the SOAP envelope
// as the root part and optional attachments. Returns the body and the
// Content-Type header value.
func mimeEncode(envelope []byte, contentType string, attachments []Attachment) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	rootID := "<soap-envelope@wsdl2go>"

	// Root part: SOAP envelope.
	rootHeader := make(textproto.MIMEHeader)
	rootHeader.Set("Content-Type", contentType)
	rootHeader.Set("Content-Id", rootID)
	rootHeader.Set("Content-Transfer-Encoding", "8bit")
	part, err := w.CreatePart(rootHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(envelope); err != nil {
		return nil, "", err
	}

	// Attachment parts.
	for _, att := range attachments {
		h := make(textproto.MIMEHeader)
		ct := att.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		h.Set("Content-Type", ct)
		h.Set("Content-Id", "<"+att.ContentID+">")
		h.Set("Content-Transfer-Encoding", "binary")
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(att.Data); err != nil {
			return nil, "", err
		}
	}

	if err := w.Close(); err != nil {
		return nil, "", err
	}

	header := fmt.Sprintf("multipart/related; type=%q; start=%q; boundary=%q",
		contentType, rootID, w.Boundary())

	return buf.Bytes(), header, nil
}

// mimeDecode parses a multipart/related response. It decodes the root
// part (SOAP envelope) into out and returns any additional parts as
// attachments.
func mimeDecode(body io.Reader, contentTypeHeader string, out Message) ([]Attachment, error) {
	_, params, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		return nil, fmt.Errorf("mime: parse content-type: %v", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("mime: missing boundary")
	}

	reader := multipart.NewReader(body, boundary)
	startID := params["start"]

	var rootPart []byte
	var attachments []Attachment

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("mime: read part: %v", err)
		}

		data, err := io.ReadAll(part)
		if err != nil {
			return nil, fmt.Errorf("mime: read part body: %v", err)
		}

		cid := part.Header.Get("Content-Id")
		ct := part.Header.Get("Content-Type")

		// Identify the root part by start parameter or by content type.
		isRoot := rootPart == nil &&
			(startID == "" || cid == startID ||
				strings.Contains(ct, "text/xml") ||
				strings.Contains(ct, "application/soap+xml") ||
				strings.Contains(ct, "application/xop+xml"))

		if isRoot {
			rootPart = data
		} else {
			attachments = append(attachments, Attachment{
				ContentID:   strings.Trim(cid, "<>"),
				ContentType: ct,
				Data:        data,
			})
		}
	}

	if rootPart == nil {
		return nil, fmt.Errorf("mime: no root SOAP part found")
	}

	// Decode the root part as a SOAP envelope.
	marshalStructure := struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    Message
	}{Body: out}

	decoder := xml.NewDecoder(bytes.NewReader(rootPart))
	decoder.CharsetReader = charset.NewReaderLabel
	if err := decoder.Decode(&marshalStructure); err != nil {
		return nil, err
	}
	return attachments, nil
}
