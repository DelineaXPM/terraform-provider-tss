package delinea

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dsschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	ephschema "github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	provschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type docAttr struct {
	name      string
	section   string
	typeLabel string
	sensitive bool
	writeOnly bool
}

type docBlock struct {
	name      string
	typeLabel string
	attrs     []docAttr
}

type docSchema struct {
	attrs  []docAttr
	blocks []docBlock
}

type schemaAttribute interface {
	IsRequired() bool
	IsOptional() bool
	IsComputed() bool
	IsSensitive() bool
	GetType() attr.Type
}

func sectionFor(a schemaAttribute) string {
	switch {
	case a.IsRequired():
		return "Required"
	case a.IsOptional():
		return "Optional"
	default:
		return "Read-Only"
	}
}

func typeLabel(t attr.Type) string {
	switch valueType := t.(type) {
	case basetypes.StringType:
		return "String"
	case basetypes.Int64Type:
		return "Number"
	case basetypes.BoolType:
		return "Boolean"
	case basetypes.ListType:
		return "List of " + typeLabel(valueType.ElemType)
	}
	return fmt.Sprintf("UNSUPPORTED(%T)", t)
}

func expectedAttr(name string, a schemaAttribute, writeOnly bool, nestedLabel string) docAttr {
	label := nestedLabel
	if label == "" {
		label = typeLabel(a.GetType())
	}
	return docAttr{name: name, section: sectionFor(a), typeLabel: label, sensitive: a.IsSensitive(), writeOnly: writeOnly}
}

func expectedFromResource(t *testing.T, r resource.Resource) docSchema {
	t.Helper()
	response := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, response)
	var out docSchema
	for name, a := range response.Schema.Attributes {
		writeOnly := false
		if s, ok := a.(resschema.StringAttribute); ok {
			writeOnly = s.WriteOnly
		}
		out.attrs = append(out.attrs, expectedAttr(name, a, writeOnly, ""))
	}
	for name, b := range response.Schema.Blocks {
		block := docBlock{name: name}
		var nested map[string]resschema.Attribute
		switch nb := b.(type) {
		case resschema.ListNestedBlock:
			block.typeLabel = "Block List"
			nested = nb.NestedObject.Attributes
		case resschema.SingleNestedBlock:
			block.typeLabel = "Block"
			nested = nb.Attributes
		default:
			t.Fatalf("unsupported block type %T for %q", b, name)
		}
		for attrName, a := range nested {
			writeOnly := false
			if s, ok := a.(resschema.StringAttribute); ok {
				writeOnly = s.WriteOnly
			}
			block.attrs = append(block.attrs, expectedAttr(attrName, a, writeOnly, ""))
		}
		out.blocks = append(out.blocks, block)
	}
	return out
}

func expectedFromDataSource(t *testing.T, d datasource.DataSource) docSchema {
	t.Helper()
	response := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, response)
	var out docSchema
	for name, a := range response.Schema.Attributes {
		if nested, ok := a.(dsschema.ListNestedAttribute); ok {
			out.attrs = append(out.attrs, expectedAttr(name, a, false, "Attributes List"))
			block := docBlock{name: name, typeLabel: "Attributes List"}
			for attrName, na := range nested.NestedObject.Attributes {
				block.attrs = append(block.attrs, expectedAttr(attrName, na, false, ""))
			}
			out.blocks = append(out.blocks, block)
			continue
		}
		out.attrs = append(out.attrs, expectedAttr(name, a, false, ""))
	}
	return out
}

func expectedFromEphemeral(t *testing.T, e ephemeral.EphemeralResource) docSchema {
	t.Helper()
	response := &ephemeral.SchemaResponse{}
	e.Schema(context.Background(), ephemeral.SchemaRequest{}, response)
	var out docSchema
	for name, a := range response.Schema.Attributes {
		if nested, ok := a.(ephschema.ListNestedAttribute); ok {
			out.attrs = append(out.attrs, expectedAttr(name, a, false, "Attributes List"))
			block := docBlock{name: name, typeLabel: "Attributes List"}
			for attrName, na := range nested.NestedObject.Attributes {
				block.attrs = append(block.attrs, expectedAttr(attrName, na, false, ""))
			}
			out.blocks = append(out.blocks, block)
			continue
		}
		out.attrs = append(out.attrs, expectedAttr(name, a, false, ""))
	}
	return out
}

func expectedFromProvider(t *testing.T, p provider.Provider) docSchema {
	t.Helper()
	response := &provider.SchemaResponse{}
	p.Schema(context.Background(), provider.SchemaRequest{}, response)
	var out docSchema
	for name, a := range response.Schema.Attributes {
		if _, ok := a.(provschema.StringAttribute); !ok {
			if _, ok := a.(provschema.BoolAttribute); !ok {
				t.Fatalf("unsupported provider attribute type %T for %q", a, name)
			}
		}
		out.attrs = append(out.attrs, expectedAttr(name, a, false, ""))
	}
	return out
}

var (
	docBulletPattern  = regexp.MustCompile("^- `([a-z_]+)` \\(([^)]*)\\)")
	nestedHeadPattern = regexp.MustCompile("^### Nested Schema for `([a-z_]+)`")
)

func parseDocSchema(t *testing.T, path string) docSchema {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}()

	var out docSchema
	inSchema := false
	section := ""
	var current *docBlock
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " ")
		switch {
		case line == "## Schema":
			inSchema = true
			continue
		case !inSchema:
			continue
		case strings.HasPrefix(line, "## "):
			inSchema = false
			continue
		}
		if m := nestedHeadPattern.FindStringSubmatch(line); m != nil {
			out.blocks = append(out.blocks, docBlock{name: m[1]})
			current = &out.blocks[len(out.blocks)-1]
			section = ""
			continue
		}
		switch strings.TrimSuffix(line, ":") {
		case "### Required", "Required":
			section = "Required"
			continue
		case "### Optional", "Optional":
			section = "Optional"
			continue
		case "### Read-Only", "Read-Only":
			section = "Read-Only"
			continue
		}
		m := docBulletPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if section == "" {
			t.Fatalf("%s: bullet %q appears outside a Required/Optional/Read-Only section", path, line)
		}
		labels := strings.Split(m[2], ", ")
		a := docAttr{name: m[1], section: section, typeLabel: labels[0]}
		for _, label := range labels[1:] {
			switch label {
			case "Sensitive":
				a.sensitive = true
			case "Write-only":
				a.writeOnly = true
			case "Min: 1":
			default:
				t.Fatalf("%s: unknown label %q on %q", path, label, line)
			}
		}
		if current != nil {
			current.attrs = append(current.attrs, a)
		} else {
			out.attrs = append(out.attrs, a)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return out
}

func compareDocAttrs(t *testing.T, where string, got, want []docAttr) {
	t.Helper()
	byName := func(attrs []docAttr) map[string]docAttr {
		m := map[string]docAttr{}
		for _, a := range attrs {
			if _, dup := m[a.name]; dup {
				t.Errorf("%s: `%s` is listed more than once", where, a.name)
			}
			m[a.name] = a
		}
		return m
	}
	gotByName, wantByName := byName(got), byName(want)
	names := map[string]bool{}
	for n := range gotByName {
		names[n] = true
	}
	for n := range wantByName {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)
	for _, n := range sorted {
		g, inDoc := gotByName[n]
		w, inSchema := wantByName[n]
		switch {
		case !inSchema:
			t.Errorf("%s: `%s` is documented but not in the schema", where, n)
		case !inDoc:
			t.Errorf("%s: `%s` (%s, %s) is in the schema but not documented", where, n, w.typeLabel, w.section)
		case g != w:
			t.Errorf("%s: `%s` documented as %+v, schema says %+v", where, n, g, w)
		}
	}
}

func compareDocSchema(t *testing.T, path string, want docSchema) {
	t.Helper()
	got := parseDocSchema(t, path)
	for i := range want.blocks {
		b := &want.blocks[i]
		if b.typeLabel != "Attributes List" {
			want.attrs = append(want.attrs, docAttr{name: b.name, section: "Optional", typeLabel: b.typeLabel})
		}
	}
	compareDocAttrs(t, path, got.attrs, want.attrs)
	gotBlocks := map[string]docBlock{}
	for _, b := range got.blocks {
		gotBlocks[b.name] = b
	}
	for _, w := range want.blocks {
		g, ok := gotBlocks[w.name]
		if !ok {
			t.Errorf("%s: missing \"### Nested Schema for `%s`\" section", path, w.name)
			continue
		}
		compareDocAttrs(t, path+" nested "+w.name, g.attrs, w.attrs)
		delete(gotBlocks, w.name)
	}
	for name := range gotBlocks {
		t.Errorf("%s: nested schema section for `%s` has no matching block or nested attribute", path, name)
	}
}

func TestDocsMatchSchemas(t *testing.T) {
	docs := filepath.Join("..", "docs")
	compareDocSchema(t, filepath.Join(docs, "index.md"), expectedFromProvider(t, New()))
	compareDocSchema(t, filepath.Join(docs, "resources", "resource_secret.md"), expectedFromResource(t, &TSSSecretResource{}))
	compareDocSchema(t, filepath.Join(docs, "resources", "secret_deletion.md"), expectedFromResource(t, &TSSSecretDeletionResource{}))
	compareDocSchema(t, filepath.Join(docs, "data-sources", "secret.md"), expectedFromDataSource(t, &TSSSecretDataSource{}))
	compareDocSchema(t, filepath.Join(docs, "data-sources", "secrets.md"), expectedFromDataSource(t, &TSSSecretsDataSource{}))
	compareDocSchema(t, filepath.Join(docs, "ephemeral-resources", "secret.md"), expectedFromEphemeral(t, &TSSSecretEphemeralResource{}))
	compareDocSchema(t, filepath.Join(docs, "ephemeral-resources", "secrets.md"), expectedFromEphemeral(t, &TSSSecretsEphemeralResource{}))
}

func TestDocsCoverEveryRegisteredType(t *testing.T) {
	ctx := context.Background()
	p := New()
	want := map[string]string{}
	for _, newResource := range p.(*TSSProvider).Resources(ctx) {
		response := &resource.MetadataResponse{}
		newResource().Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "tss"}, response)
		want[strings.TrimPrefix(response.TypeName, "tss_")] = "resources"
	}
	for _, newDataSource := range p.(*TSSProvider).DataSources(ctx) {
		response := &datasource.MetadataResponse{}
		newDataSource().Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "tss"}, response)
		want[strings.TrimPrefix(response.TypeName, "tss_")+"@data-sources"] = "data-sources"
	}
	for _, newEphemeral := range p.(*TSSProvider).EphemeralResources(ctx) {
		response := &ephemeral.MetadataResponse{}
		newEphemeral().Metadata(ctx, ephemeral.MetadataRequest{ProviderTypeName: "tss"}, response)
		want[strings.TrimPrefix(response.TypeName, "tss_")+"@ephemeral-resources"] = "ephemeral-resources"
	}
	for key, dir := range want {
		name := strings.SplitN(key, "@", 2)[0]
		path := filepath.Join("..", "docs", dir, name+".md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("registered type tss_%s has no docs page at %s", name, path)
		}
	}
}
