package api

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"google.golang.org/protobuf/proto"
)

// Object is the interface implemented by every API object; that is to say, it
// is a protobuf message with a standard Metadata field which can be redacted.
type Object interface {
	proto.Message
	GetMetadata() *Metadata
	Redact()
}

// NewMetadata builds a Metadata whose Type is derived from T's proto full
// name, so callers never need to hardcode type strings or pass a nil pointer.
//
//	api.NewMetadata[*awsap.AgentPool]("default")
func NewMetadata[T proto.Message](id string) *Metadata {
	var zero T
	typeName := string(zero.ProtoReflect().Descriptor().FullName())
	return Metadata_builder{
		Type: to.Ptr(typeName),
		Id:   to.Ptr(id),
	}.Build()
}
