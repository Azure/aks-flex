package api

import (
	"google.golang.org/protobuf/proto"
)

// Object is the interface implemented by every API object; that is to say, it
// is a protobuf message with a standard Metadata field which can be redacted.
type Object interface {
	proto.Message
	GetMetadata() *Metadata
	Redact()
}
