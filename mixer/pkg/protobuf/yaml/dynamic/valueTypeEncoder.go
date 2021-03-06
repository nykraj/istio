// Copyright 2018 Istio Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dynamic

import (
	"fmt"

	"github.com/gogo/protobuf/protoc-gen-gogo/descriptor"

	"istio.io/api/policy/v1beta1"
	"istio.io/istio/mixer/pkg/attribute"
	"istio.io/istio/mixer/pkg/lang/compiled"
)

// valueTypeName is the Name of the value type with special input encoding
const valueTypeName = ".istio.policy.v1beta1.Value"

func valueTypeEncoderBuilder(_ *descriptor.DescriptorProto, fd *descriptor.FieldDescriptorProto, v interface{}, compiler Compiler) (Encoder, error) {
	if fd.GetTypeName() != valueTypeName {
		return nil, fmt.Errorf("cannot process message of type:%s", fd.GetTypeName())
	}

	var vVal v1beta1.Value
	switch vv := v.(type) {
	case int:
		vVal.Value = &v1beta1.Value_Int64Value{int64(vv)}
	case float64:
		vVal.Value = &v1beta1.Value_DoubleValue{vv}
	case bool:
		vVal.Value = &v1beta1.Value_BoolValue{vv}
	case string:
		val, isConstString := transformQuotedString(vv)
		sval, _ := val.(string)
		if isConstString {
			vVal.Value = &v1beta1.Value_StringValue{sval}
		} else {
			var expr compiled.Expression
			var vt v1beta1.ValueType
			var err error

			if expr, vt, err = compiler.Compile(sval); err != nil {
				return nil, err
			}

			var enc populateValueFun
			switch vt {
			case v1beta1.STRING:
				enc = func(bag attribute.Bag, vVal *v1beta1.Value) error {
					ev, er := expr.EvaluateString(bag)
					if er != nil {
						return er
					}
					vVal.Value = &v1beta1.Value_StringValue{ev}
					return nil
				}
			case v1beta1.BOOL:
				enc = func(bag attribute.Bag, vVal *v1beta1.Value) error {
					ev, er := expr.EvaluateBoolean(bag)
					if er != nil {
						return er
					}
					vVal.Value = &v1beta1.Value_BoolValue{ev}
					return nil
				}
			case v1beta1.INT64:
				enc = func(bag attribute.Bag, vVal *v1beta1.Value) error {
					ev, er := expr.EvaluateInteger(bag)
					if er != nil {
						return er
					}
					vVal.Value = &v1beta1.Value_Int64Value{ev}
					return nil
				}
			case v1beta1.DOUBLE:
				enc = func(bag attribute.Bag, vVal *v1beta1.Value) error {
					ev, er := expr.EvaluateDouble(bag)
					if er != nil {
						return er
					}
					vVal.Value = &v1beta1.Value_DoubleValue{ev}
					return nil
				}
			default:
				return nil, fmt.Errorf("unsupported type: %v", vt)
			}
			return &valueTypeDynamicEncoder{enc}, nil
		}
	default:
		return nil, fmt.Errorf("unsupported type: %T of %v", v, v)
	}

	encodedData := make([]byte, 0, vVal.Size()+2)
	var err error
	if encodedData, err = marshalValWithSize(&vVal, encodedData); err != nil {
		return nil, err
	}

	return &staticEncoder{
		name:        fd.GetName(),
		encodedData: encodedData,
	}, nil
}

type populateValueFun func(bag attribute.Bag, vVal *v1beta1.Value) error

type valueTypeDynamicEncoder struct {
	populateValue populateValueFun
}

func (vv valueTypeDynamicEncoder) Encode(bag attribute.Bag, ba []byte) ([]byte, error) {
	var vVal v1beta1.Value
	if err := vv.populateValue(bag, &vVal); err != nil {
		return nil, err
	}

	return marshalValWithSize(&vVal, ba)
}

func marshalValWithSize(vVal *v1beta1.Value, ba []byte) ([]byte, error) {
	msgSize := vVal.Size()
	ba, _ = EncodeVarint(ba, uint64(msgSize))
	startIdx := len(ba)
	ba = extendSlice(ba, msgSize)
	_, err := vVal.MarshalTo(ba[startIdx:])
	return ba, err
}
