# wsdl2go

[![CI](https://github.com/fiorix/wsdl2go/actions/workflows/ci.yml/badge.svg)](https://github.com/fiorix/wsdl2go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/fiorix/wsdl2go)](https://goreportcard.com/report/github.com/fiorix/wsdl2go)
[![Go Reference](https://pkg.go.dev/badge/github.com/fiorix/wsdl2go.svg)](https://pkg.go.dev/github.com/fiorix/wsdl2go)

wsdl2go is a command line tool to generate [Go](https://golang.org) code
from [WSDL](https://en.wikipedia.org/wiki/Web_Services_Description_Language).

Download:

```
go install github.com/fiorix/wsdl2go@latest
```

### Usage

tl;dr

```
wsdl2go -i file.wsdl -o hello.go
```

wsdl2go is a code generator that consumes WSDL from stdin (or file, or URL) and produces Go on stdout. The generated code contains services and methods described in the WSDL input, in a single output file. It is your responsibility to make it a package, in the sense that you put it in a directory that makes sense for you, and import it in your code later. Note that the generated code depends on the "soap" package that is part of this project.

WSDL inputs that contain import tags (includes) pointing to other WSDL resources (other files or URLs) may be a source of trouble. The default behavior of wsdl2go is to try and load them, recursively. However, wsdl2go does not support authentication for remote HTTP resources, and cannot fetch resources from HTTPS servers with insecure TLS certificates. In those cases, you have to download the WSDL files yourself using curl or whatever, and process them locally. You might have to tweak their import paths.

Once the code is generated, wsd2go runs gofmt on it. You must have gofmt in your $PATH, or $GOROOT/bin, or you'll get an error.

### Using the generated code

Here's how to use the generated code: let's say you have a WSDL that defines the "example" service. You generate the code and make it the "example" package somewhere in your $GOPATH. This service provides an Echo method that takes an EchoRequest and returns an EchoReply.

The process is this:

- Import the generated code
- Create a soap.Client (and here you can configure SOAP authentication, for example)
- Instantiate the service using your soap.Client
- Call the service methods

Example:

```go
import (
	"/path/to/generated/example"

	"github.com/fiorix/wsdl2go/soap"
)

func main() {
	cli := soap.Client{
		URL: "http://server",
		Namespace: example.Namespace,
	}
	soapService := example.NewEchoService(&cli)
	echoReply, err := soapService.Echo(&example.EchoRequest{Data: "hello world"})
	...
}
```

The soap.Client supports two forms of authentication:

- Setting the "Pre" hook to a function that is run on all outbound HTTP requests, which can set HTTP headers and Basic Auth
- Setting the Header attribute to an AuthHeader, to have it as a SOAP header (with username and password) in every request

### Features

**SOAP protocol:**

- SOAP 1.1 and 1.2
- Document and RPC binding styles
- SOAP fault detection and typed errors (`*soap.Fault`)
- MTOM/XOP binary optimization
- MIME multipart attachments (SwA)
- Pre/Post request hooks for custom headers, auth, logging
- Per-method context support (`OpContext(ctx, ...)` variants)

**Code generation:**

- Multiple PortTypes per WSDL
- Typed enum constants (from `xs:restriction` with `xs:enumeration`)
- Recursive schema imports and includes, with download caching
- Client TLS certificates (`-cert`, `-key` flags)

**XSD types:**

| XSD | Go | Notes |
|---|---|---|
| byte, unsignedByte | byte | |
| short | int16 | |
| int | int | |
| integer | int64 | |
| long | int64 | |
| float, double, decimal | float64 | |
| boolean | bool | |
| string, token, anyURI, QName, language, id, nmtoken, normalizedString | string | |
| hexBinary, base64Binary | []byte | |
| date | soap.Date | XML marshal/unmarshal, timezone-aware |
| time | soap.Time | XML marshal/unmarshal, timezone-aware |
| dateTime | soap.DateTime | XML marshal/unmarshal, timezone-aware |
| duration | soap.Duration | String type (XSD durations have variable-length months/years) |
| nonNegativeInteger | uint | |
| positiveInteger | uint64 | |
| unsignedInt | uint | |
| anyType, anySimpleType, anySequence | *soap.AnyType | Preserves raw XML via InnerXML |
| simpleType | typed alias | With enum constants if restrictions defined |
| complexType | struct | Including extensions, sequences, choices, attributes |

**Not yet supported:** decimal as distinct type, g{Day,Month,Year,...}, NOTATION, xs:group, xs:list.

Because of some [limitations](https://github.com/golang/go/issues/14407) of XML namespaces in Go, there are constraints on what SOAP can do correctly. The generated code works with most enterprise SOAP systems.
