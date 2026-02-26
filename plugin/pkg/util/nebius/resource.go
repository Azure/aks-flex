package nebius

import (
	"context"
	"fmt"
	"reflect"

	nebiuscommon "github.com/nebius/gosdk/proto/nebius/common/v1"
)

// Resource represents a cloud resource from nebius.
type Resource[S any] interface {
	GetMetadata() *nebiuscommon.ResourceMetadata
	GetSpec() S
}

// ResourceDrift represents the drift status of a resource.
type ResourceDrift[T Resource[S], S any] struct {
	// HasDrift indicates whether the resource has drift or not.
	// If true, the resource has drift and the Desired field contains the desired state of the resource.
	// Otherwise, the resource is in sync and the Desired field is empty.
	HasDrift bool
	// Desired contains the desired state of the resource if it has drift.
	Desired T
}

type DriftDetectFunc[T Resource[S], S any] func(ctx context.Context, existing, desired T) (ResourceDrift[T, S], error)

func DriftTODO[T Resource[S], S any](ctx context.Context, existing, desired T) (ResourceDrift[T, S], error) {
	return ResourceDrift[T, S]{
		HasDrift: false,
	}, nil
}

type ResourceCRUD[T Resource[S], S any] struct {
	getByName func(ctx context.Context, t T) (T, error)
	getById   func(ctx context.Context, t T) (T, error)
	create    func(ctx context.Context, t T) (T, error)
	update    func(ctx context.Context, t T) (T, error)
	delete    func(ctx context.Context, t T) error
}

// ResourceCRUDFactory creates a factory method for a nebius resource from a given service client.
func ResourceCRUDFactory[SVC any, T Resource[S], S any]() func(s SVC) *ResourceCRUD[T, S] {
	// Reflect once on the *static* type SVC.
	svcType := reflect.TypeOf((*SVC)(nil)).Elem()
	if svcType == nil {
		panic("unable to determine service type")
	}

	// Cache methods (reflection done ONCE here).
	getByNameM := expectRPCMethod(svcType, "GetByName")
	getByIDM := expectRPCMethod(svcType, "Get")
	createM := expectRPCMethod(svcType, "Create")
	updateM := expectRPCMethod(svcType, "Update")
	deleteM := expectRPCMethod(svcType, "Delete")

	// Cache request element types so we can allocate requests quickly.
	getByNameReqElem := getByNameM.Type.In(1).Elem()
	getByIDReqElem := getByIDM.Type.In(1).Elem()
	createReqElem := createM.Type.In(1).Elem()
	updateReqElem := updateM.Type.In(1).Elem()
	deleteReqElem := deleteM.Type.In(1).Elem()

	waitAndResourceID := func(ctx context.Context, opVal reflect.Value) (string, error) {
		waitM := opVal.MethodByName("Wait")
		if !waitM.IsValid() {
			return "", fmt.Errorf("operation type %v missing Wait(ctx)", opVal.Type())
		}
		waitOuts := waitM.Call([]reflect.Value{reflect.ValueOf(ctx)})
		if len(waitOuts) != 2 {
			return "", fmt.Errorf("Wait must return (op, err)")
		}
		if !waitOuts[1].IsNil() {
			return "", waitOuts[1].Interface().(error)
		}
		op2 := waitOuts[0]

		resIDM := op2.MethodByName("ResourceID")
		if !resIDM.IsValid() {
			return "", fmt.Errorf("operation type %v missing ResourceID()", op2.Type())
		}
		idOuts := resIDM.Call(nil)
		if len(idOuts) != 1 || idOuts[0].Kind() != reflect.String {
			return "", fmt.Errorf("ResourceID() must return string")
		}
		return idOuts[0].String(), nil
	}

	return func(s SVC) *ResourceCRUD[T, S] {
		svcVal := reflect.ValueOf(s)
		if !svcVal.IsValid() {
			panic("service value is invalid")
		}
		// Ensure the runtime value matches the static SVC type.
		if svcValType := svcVal.Type(); !svcValType.AssignableTo(svcType) {
			panic(fmt.Sprintf("service value type %v doesn't implement %v", svcValType, svcType))
		}

		callUnary := func(ctx context.Context, m reflect.Method, reqPtr reflect.Value) (reflect.Value, error) {
			fn := svcVal.Method(m.Index)
			if !fn.IsValid() {
				return reflect.Value{}, fmt.Errorf("method %s not found on %T", m.Name, s)
			}
			outs := fn.Call([]reflect.Value{reflect.ValueOf(ctx), reqPtr})
			if !outs[1].IsNil() {
				return reflect.Value{}, outs[1].Interface().(error)
			}
			return outs[0], nil
		}
		getByIDFromID := func(ctx context.Context, id string) (T, error) {
			req := reflect.New(getByIDReqElem)
			setField(req, "Id", id)

			out0, err := callUnary(ctx, getByIDM, req)
			if err != nil {
				var zero T
				return zero, err
			}
			return out0.Interface().(T), nil
		}

		return &ResourceCRUD[T, S]{
			getByName: func(ctx context.Context, desired T) (T, error) {
				req := reflect.New(getByNameReqElem)
				md := desired.GetMetadata()
				setField(req, "Name", md.GetName())
				setField(req, "ParentId", md.GetParentId())

				out0, err := callUnary(ctx, getByNameM, req)
				if err != nil {
					var zero T
					return zero, err
				}
				return out0.Interface().(T), nil
			},
			getById: func(ctx context.Context, desired T) (T, error) {
				req := reflect.New(getByIDReqElem)
				setField(req, "Id", desired.GetMetadata().GetId())

				out0, err := callUnary(ctx, getByIDM, req)
				if err != nil {
					var zero T
					return zero, err
				}
				return out0.Interface().(T), nil
			},
			create: func(ctx context.Context, desired T) (T, error) {
				req := reflect.New(createReqElem)
				setField(req, "Metadata", desired.GetMetadata())
				setField(req, "Spec", desired.GetSpec())

				op, err := callUnary(ctx, createM, req)
				if err != nil {
					var zero T
					return zero, err
				}
				id, err := waitAndResourceID(ctx, op)
				if err != nil {
					var zero T
					return zero, err
				}
				return getByIDFromID(ctx, id)
			},
			update: func(ctx context.Context, desired T) (T, error) {
				req := reflect.New(updateReqElem)
				setField(req, "Metadata", desired.GetMetadata())
				setField(req, "Spec", desired.GetSpec())

				op, err := callUnary(ctx, updateM, req)
				if err != nil {
					var zero T
					return zero, err
				}
				id, err := waitAndResourceID(ctx, op)
				if err != nil {
					var zero T
					return zero, err
				}
				return getByIDFromID(ctx, id)
			},
			delete: func(ctx context.Context, desired T) error {
				req := reflect.New(deleteReqElem)
				setField(req, "Id", desired.GetMetadata().GetId())
				op, err := callUnary(ctx, deleteM, req)
				if err != nil {
					// TODO: ignore not found?
					return err
				}
				_, err = waitAndResourceID(ctx, op)
				return err
			},
		}
	}
}

func (res *ResourceCRUD[T, S]) CreateOrUpdate(ctx context.Context, detectDrift DriftDetectFunc[T, S], desired T) (T, error) {
	var existing T
	var err error
	if id := desired.GetMetadata().GetId(); id != "" {
		existing, err = res.getById(ctx, desired)
	} else {
		// If ID is not set, attempt to resolve by name (assuming name is unique within the parent scope)
		existing, err = res.getByName(ctx, desired)
	}
	switch {
	case isNotFound(err):
		// Resource doesn't exist, will create
		return res.create(ctx, desired)
	case err != nil:
		// Some other error occurred
		var zero T
		return zero, err
	default:
		// Resource exists, will check if update is needed
		drifted, err := detectDrift(ctx, existing, desired)
		if err != nil {
			var zero T
			return zero, err
		}
		if !drifted.HasDrift {
			// No drift, return existing
			return existing, nil
		}
		return res.update(ctx, drifted.Desired)
	}
}

func (res *ResourceCRUD[T, S]) Delete(ctx context.Context, t T) error {
	if id := t.GetMetadata().GetId(); id != "" {
		// resource has id, delete by id directly
		return res.delete(ctx, t)
	}

	// resource doesn't have id, attempt to resolve by name first
	existing, err := res.getByName(ctx, t)
	if err != nil {
		if isNotFound(err) {
			// resource doesn't exist, consider it deleted
			return nil
		}
		return err
	}

	return res.delete(ctx, existing)
}

func expectRPCMethod(svcType reflect.Type, name string) reflect.Method {
	m, ok := svcType.MethodByName(name)
	if !ok {
		panic(fmt.Sprintf("service type %v does not have %s method", svcType, name))
	}
	// Expect: func(context.Context, *Req, ...grpc.Option) (Resp, error)
	if m.Type.NumIn() < 3 {
		panic(fmt.Sprintf("method %s on %v has unexpected signature", name, svcType))
	}
	reqTy := m.Type.In(1)
	if reqTy.Kind() != reflect.Ptr || reqTy.Elem().Kind() != reflect.Struct {
		panic(fmt.Sprintf("method %s on %v req must be *struct, got %v", name, svcType, reqTy))
	}
	if m.Type.NumOut() != 2 {
		panic(fmt.Sprintf("method %s on %v must return (resp, err)", name, svcType))
	}
	return m
}

func setField(reqPtr reflect.Value, field string, v any) {
	f := reqPtr.Elem().FieldByName(field)
	if !f.IsValid() || !f.CanSet() {
		return
	}
	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return
	}
	if val.Type().AssignableTo(f.Type()) {
		f.Set(val)
		return
	}
	if val.Type().ConvertibleTo(f.Type()) {
		f.Set(val.Convert(f.Type()))
		return
	}
}
