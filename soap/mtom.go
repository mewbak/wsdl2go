package soap

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"

	"golang.org/x/net/html/charset"
)

const xopNamespace = "http://www.w3.org/2004/08/xop/include"

// mtomEncode takes a marshaled SOAP envelope, extracts base64-encoded
// binary content, replaces it with XOP Include references, and returns
// a multipart/related MTOM message.
func mtomEncode(envelope []byte, soapContentType string) ([]byte, string, error) {
	// Parse the XML to find base64Binary elements and extract them.
	decoder := xml.NewDecoder(bytes.NewReader(envelope))
	decoder.CharsetReader = charset.NewReaderLabel

	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	var attachments []Attachment
	cidCounter := 0

	// Track element depth and whether we're inside a base64 element.
	type frame struct {
		token   xml.StartElement
		content bytes.Buffer
		isLeaf  bool
	}
	var stack []*frame

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, &frame{token: t, isLeaf: true})
			if err := encoder.EncodeToken(xml.CopyToken(t)); err != nil {
				return nil, "", err
			}

		case xml.EndElement:
			if len(stack) > 0 {
				f := stack[len(stack)-1]
				stack = stack[:len(stack)-1]

				replaced := false

				// If this is a leaf element with substantial base64 content,
				// replace it with an XOP Include reference.
				if f.isLeaf && f.content.Len() > 512 {
					raw := strings.TrimSpace(f.content.String())
					data, decErr := base64.StdEncoding.DecodeString(raw)
					if decErr == nil && len(data) > 0 {
						cid := fmt.Sprintf("part%d@wsdl2go", cidCounter)
						cidCounter++

						attachments = append(attachments, Attachment{
							ContentID:   cid,
							ContentType: "application/octet-stream",
							Data:        data,
						})

						include := xml.StartElement{
							Name: xml.Name{Space: xopNamespace, Local: "Include"},
							Attr: []xml.Attr{
								{Name: xml.Name{Local: "href"}, Value: "cid:" + cid},
							},
						}
						encoder.EncodeToken(include)
						encoder.EncodeToken(include.End())
						replaced = true
					}
				}

				// Write back captured content if it wasn't replaced.
				if f.isLeaf && !replaced && f.content.Len() > 0 {
					encoder.EncodeToken(xml.CharData(f.content.Bytes()))
				}

				// Mark parent as non-leaf since it has child elements.
				if len(stack) > 0 {
					stack[len(stack)-1].isLeaf = false
				}
			}
			if err := encoder.EncodeToken(xml.CopyToken(t)); err != nil {
				return nil, "", err
			}

		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1].isLeaf {
				stack[len(stack)-1].content.Write(t)
				// Don't write CharData yet; we might replace it with XOP Include.
				// Only write if the element turns out not to be binary.
			}
			// For non-leaf or no stack, write through.
			if len(stack) == 0 || !stack[len(stack)-1].isLeaf {
				if err := encoder.EncodeToken(xml.CopyToken(t)); err != nil {
					return nil, "", err
				}
			}

		default:
			if err := encoder.EncodeToken(xml.CopyToken(tok)); err != nil {
				return nil, "", err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, "", err
	}

	if len(attachments) == 0 {
		// No binary content extracted; return original envelope as-is
		// in a MIME wrapper for consistency.
		return mtomWrap(envelope, soapContentType, nil)
	}

	// Write any non-replaced CharData for leaf elements that weren't binary.
	// Actually, we need to handle the case where leaf elements have content
	// that isn't base64. This is handled by the threshold check above.

	return mtomWrap(buf.Bytes(), soapContentType, attachments)
}

func mtomWrap(envelope []byte, soapContentType string, attachments []Attachment) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	rootID := "<soap-envelope@wsdl2go>"
	xopContentType := fmt.Sprintf("application/xop+xml; charset=utf-8; type=%q", soapContentType)

	rootHeader := make(textproto.MIMEHeader)
	rootHeader.Set("Content-Type", xopContentType)
	rootHeader.Set("Content-Id", rootID)
	rootHeader.Set("Content-Transfer-Encoding", "8bit")
	part, err := w.CreatePart(rootHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(envelope); err != nil {
		return nil, "", err
	}

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

	header := fmt.Sprintf("multipart/related; type=%q; start=%q; start-info=%q; boundary=%q",
		"application/xop+xml", rootID, soapContentType, w.Boundary())

	return buf.Bytes(), header, nil
}

// mtomDecode parses an MTOM multipart response. It resolves XOP Include
// references by replacing them with the binary content from the
// corresponding MIME parts, then decodes the SOAP envelope.
func mtomDecode(body io.Reader, contentTypeHeader string, out Message) error {
	_, params, err := mime.ParseMediaType(contentTypeHeader)
	if err != nil {
		return fmt.Errorf("mtom: parse content-type: %v", err)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return fmt.Errorf("mtom: missing boundary")
	}

	reader := multipart.NewReader(body, boundary)
	startID := params["start"]

	var rootPart []byte
	parts := make(map[string][]byte) // Content-ID -> data

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("mtom: read part: %v", err)
		}

		data, err := io.ReadAll(part)
		if err != nil {
			return fmt.Errorf("mtom: read part body: %v", err)
		}

		cid := part.Header.Get("Content-Id")
		ct := part.Header.Get("Content-Type")

		isRoot := rootPart == nil &&
			(startID == "" || cid == startID ||
				strings.Contains(ct, "application/xop+xml"))

		if isRoot {
			rootPart = data
		} else {
			// Strip angle brackets from Content-ID for lookup.
			parts[strings.Trim(cid, "<>")] = data
		}
	}

	if rootPart == nil {
		return fmt.Errorf("mtom: no root XOP part found")
	}

	// Resolve XOP Include references by replacing them with base64-encoded
	// content from the MIME parts.
	resolved, err := resolveXOPIncludes(rootPart, parts)
	if err != nil {
		return err
	}

	marshalStructure := struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    Message
	}{Body: out}

	decoder := xml.NewDecoder(bytes.NewReader(resolved))
	decoder.CharsetReader = charset.NewReaderLabel
	return decoder.Decode(&marshalStructure)
}

// resolveXOPIncludes replaces <xop:Include href="cid:..."/> elements
// with the base64-encoded content from the corresponding MIME parts.
func resolveXOPIncludes(xmlData []byte, parts map[string][]byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(xmlData))
	decoder.CharsetReader = charset.NewReaderLabel

	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space == xopNamespace && t.Name.Local == "Include" {
				// Find the href attribute.
				var href string
				for _, attr := range t.Attr {
					if attr.Name.Local == "href" {
						href = attr.Value
						break
					}
				}

				// Extract Content-ID from "cid:..." href.
				cid := strings.TrimPrefix(href, "cid:")
				data, ok := parts[cid]
				if !ok {
					return nil, fmt.Errorf("mtom: missing part for Content-ID %q", cid)
				}

				// Write base64-encoded content in place of the Include element.
				encoded := base64.StdEncoding.EncodeToString(data)
				encoder.EncodeToken(xml.CharData([]byte(encoded)))

				// Skip the closing </xop:Include> element.
				decoder.Skip()
				continue
			}
			if err := encoder.EncodeToken(xml.CopyToken(t)); err != nil {
				return nil, err
			}

		default:
			if err := encoder.EncodeToken(xml.CopyToken(tok)); err != nil {
				return nil, err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
