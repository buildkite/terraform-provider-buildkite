package buildkite

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Helpers for driving resources directly, without the acceptance test harness and the live
// organization it needs.

// nullObjectWith builds a value of schemaType in which every attribute is null except those in set.
// Unit tests care about a handful of attributes, and the schemas here have dozens; a null default
// keeps a test's setup to the attributes that actually drive the code path under test.
func nullObjectWith(ctx context.Context, t *testing.T, schemaType attr.Type, set map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	objectType, ok := schemaType.TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("Expected the schema to be an object")
	}

	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attributeType, nil)
	}
	for name, value := range set {
		if _, ok := attributes[name]; !ok {
			t.Fatalf("Attribute %q is not in the schema", name)
		}
		attributes[name] = value
	}

	return tftypes.NewValue(objectType, attributes)
}

// diagnosticsContain reports whether any error diagnostic mentions summary, so a test can assert on
// the failure it set up without depending on the full diagnostic text or on the count, which a
// method driven outside the framework picks up extras from.
func diagnosticsContain(diags diag.Diagnostics, summary string) bool {
	for _, d := range diags.Errors() {
		if strings.Contains(d.Summary(), summary) || strings.Contains(d.Detail(), summary) {
			return true
		}
	}

	return false
}
