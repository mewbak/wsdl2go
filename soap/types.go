package soap

// AnyType represents an xsd:anyType element, preserving its raw XML content.
type AnyType struct {
	InnerXML string `xml:",innerxml"`
}
