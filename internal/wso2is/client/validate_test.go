/*
 * Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package client

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/getkin/kin-openapi/openapi3"
)

func buildValidationTestOp() *catalog.Operation {
	bodySchema := openapi3.NewObjectSchema()
	bodySchema.Required = []string{"userName"}
	bodySchema.Properties = openapi3.Schemas{
		"userName": openapi3.NewSchemaRef("", openapi3.NewStringSchema()),
		"active":   openapi3.NewSchemaRef("", openapi3.NewBoolSchema()),
		"age":      openapi3.NewSchemaRef("", openapi3.NewIntegerSchema()),
	}

	return &catalog.Operation{
		ID:     "users.updateUser",
		Method: http.MethodPut,
		Path:   "/scim2/Users/{id}",
		Kind:   catalog.KindWrite,
		Params: []catalog.Param{
			{Name: "id", In: "path", Required: true, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
			{Name: "count", In: "query", Required: false, Schema: openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())},
			{Name: "active", In: "query", Required: false, Schema: openapi3.NewSchemaRef("", openapi3.NewBoolSchema())},
			{Name: "filter", In: "query", Required: false, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
		},
		Body: &catalog.BodySpec{
			ContentType: "application/scim+json",
			Required:    true,
			Schema:      openapi3.NewSchemaRef("", bodySchema),
		},
	}
}

func TestValidateInput_MissingPathParam(t *testing.T) {
	op := buildValidationTestOp()
	err := ValidateInput(op, nil, nil, map[string]any{"userName": "alice"})
	if err == nil {
		t.Fatal("expected error for missing path param")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "invalid input: ") {
		t.Errorf("error = %q, want prefix 'invalid input: '", msg)
	}
	if !strings.Contains(msg, "id") {
		t.Errorf("error = %q, want field name 'id'", msg)
	}
}

func TestValidateInput_EmptyPathParam(t *testing.T) {
	op := buildValidationTestOp()
	err := ValidateInput(op, map[string]string{"id": ""}, nil, map[string]any{"userName": "alice"})
	if err == nil {
		t.Fatal("expected error for empty path param")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Errorf("error = %q, want field name 'id'", err.Error())
	}
}

func TestValidateInput_WrongQueryParamTypes(t *testing.T) {
	op := buildValidationTestOp()

	cases := []struct {
		name  string
		query map[string]any
		field string
	}{
		{
			name:  "string for integer",
			query: map[string]any{"count": "not-an-int"},
			field: "count",
		},
		{
			name:  "bool for integer",
			query: map[string]any{"count": true},
			field: "count",
		},
		{
			name:  "string for boolean",
			query: map[string]any{"active": "not-a-bool"},
			field: "active",
		},
		{
			name:  "number for string",
			query: map[string]any{"filter": 123},
			field: "filter",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateInput(op, map[string]string{"id": "u1"}, tc.query, map[string]any{"userName": "alice"})
			if err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
			msg := err.Error()
			if !strings.HasPrefix(msg, "invalid input: ") {
				t.Errorf("error = %q, want prefix 'invalid input: '", msg)
			}
			if !strings.Contains(msg, tc.field) {
				t.Errorf("error = %q, want field %q", msg, tc.field)
			}
		})
	}
}

func TestValidateInput_UnknownQueryParam(t *testing.T) {
	op := buildValidationTestOp()
	err := ValidateInput(op, map[string]string{"id": "u1"}, map[string]any{"unknownParam": "x"}, map[string]any{"userName": "alice"})
	if err == nil {
		t.Fatal("expected validation error for unknown query param")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "invalid input: ") {
		t.Errorf("error = %q, want prefix 'invalid input: '", msg)
	}
	if !strings.Contains(msg, "unknownParam") {
		t.Errorf("error = %q, want field 'unknownParam'", msg)
	}
}

func TestValidateInput_InvalidBody(t *testing.T) {
	op := buildValidationTestOp()

	t.Run("missing required body", func(t *testing.T) {
		err := ValidateInput(op, map[string]string{"id": "u1"}, nil, nil)
		if err == nil {
			t.Fatal("expected error for missing required body")
		}
		if !strings.HasPrefix(err.Error(), "invalid input: ") || !strings.Contains(err.Error(), "body") {
			t.Errorf("unexpected error: %q", err.Error())
		}
	})

	t.Run("missing required property", func(t *testing.T) {
		err := ValidateInput(op, map[string]string{"id": "u1"}, nil, map[string]any{"active": true})
		if err == nil {
			t.Fatal("expected error for missing userName")
		}
		if !strings.Contains(err.Error(), "userName") {
			t.Errorf("error = %q, want field 'userName'", err.Error())
		}
	})

	t.Run("wrong property type", func(t *testing.T) {
		err := ValidateInput(op, map[string]string{"id": "u1"}, nil, map[string]any{
			"userName": "alice",
			"age":      "twenty",
		})
		if err == nil {
			t.Fatal("expected error for wrong age type")
		}
		if !strings.Contains(err.Error(), "age") {
			t.Errorf("error = %q, want field 'age'", err.Error())
		}
	})

	t.Run("body passed when spec defines none", func(t *testing.T) {
		getOp := &catalog.Operation{
			ID:     "users.getUser",
			Method: http.MethodGet,
			Path:   "/scim2/Users/{id}",
			Params: []catalog.Param{
				{Name: "id", In: "path", Required: true, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
			},
		}
		err := ValidateInput(getOp, map[string]string{"id": "u1"}, nil, map[string]any{"extra": 1})
		if err == nil {
			t.Fatal("expected error when body supplied to bodyless operation")
		}
		if !strings.Contains(err.Error(), "body") {
			t.Errorf("error = %q, want 'body'", err.Error())
		}
	})
}

func TestValidateInput_ValidAndMultipart(t *testing.T) {
	op := buildValidationTestOp()
	err := ValidateInput(op,
		map[string]string{"id": "u1"},
		map[string]any{"count": 10, "active": true, "filter": "userName eq alice"},
		map[string]any{"userName": "alice", "active": true, "age": 30},
	)
	if err != nil {
		t.Fatalf("expected valid input, got: %v", err)
	}

	fileSchema := openapi3.NewStringSchema()
	fileSchema.Format = "binary"
	obj := openapi3.NewObjectSchema().WithProperty("file", fileSchema)
	obj.Required = []string{"file"}
	mpOp := &catalog.Operation{
		ID:     "applications.importApplication",
		Method: http.MethodPost,
		Path:   "/api/server/v1/applications/import",
		Body: &catalog.BodySpec{
			ContentType: "multipart/form-data",
			Required:    true,
			Schema:      openapi3.NewSchemaRef("", obj),
		},
	}

	if err := ValidateInput(mpOp, nil, nil, map[string]any{
		"file": base64.StdEncoding.EncodeToString([]byte("hello")),
	}); err != nil {
		t.Fatalf("expected valid multipart base64 input, got: %v", err)
	}

	if err := ValidateInput(mpOp, nil, nil, map[string]any{
		"file": "!!!not-base64!!!",
	}); err == nil || !strings.Contains(err.Error(), "file") {
		t.Fatalf("expected error mentioning 'file' for bad base64, got: %v", err)
	}
}

func TestValidateInput_RealCatalogOperation(t *testing.T) {
	cat, err := catalog.Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("catalog.Load failed: %v", err)
	}

	op, ok := cat.Get("users.getUserById")
	if !ok {
		t.Fatal("expected users.getUserById in catalog")
	}
	if err := ValidateInput(op, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "id") {
		t.Errorf("expected missing id error, got: %v", err)
	}
	if err := ValidateInput(op, map[string]string{"id": "u-1"}, nil, nil); err != nil {
		t.Errorf("expected valid users.getUserById input, got: %v", err)
	}

	listOp, ok := cat.Get("users.getUser")
	if !ok {
		t.Fatal("expected users.getUser in catalog")
	}
	if err := ValidateInput(listOp, nil, nil, nil); err != nil {
		t.Errorf("expected valid users.getUser input, got: %v", err)
	}

	createOp, ok := cat.Get("users.createUser")
	if !ok {
		t.Fatal("expected users.createUser in catalog")
	}
	if err := ValidateInput(createOp, nil, nil, map[string]any{"userName": "alice"}); err != nil {
		t.Errorf("expected valid users.createUser input, got: %v", err)
	}
}
