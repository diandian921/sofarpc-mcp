package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/diandian921/sofarpc-mcp/skills"
)

// TestEmbeddedSkillEntry pins that the shipped SKILL.md is a valid skill: its
// frontmatter name matches the skill:// path and its single resource describes the
// exact embedded bytes.
func TestEmbeddedSkillEntry(t *testing.T) {
	entry, err := newSkillEntry(usingSofaRPCSkillName, usingSofaRPCSkillURI, skills.UsingSofaRPC)
	if err != nil {
		t.Fatalf("embedded skill is invalid: %v", err)
	}
	if entry.URI != "skill://using-sofarpc/SKILL.md" {
		t.Errorf("uri = %q", entry.URI)
	}
	if len(entry.Frontmatter) != 2 {
		t.Errorf("frontmatter = %v, want exactly name and description", entry.Frontmatter)
	}
	if len(entry.Resources) != 1 {
		t.Fatalf("resources = %+v, want one entry", entry.Resources)
	}
	sum := sha256.Sum256(skills.UsingSofaRPC)
	want := skillResource{
		URI:    entry.URI,
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
		Size:   len(skills.UsingSofaRPC),
	}
	if entry.Resources[0] != want {
		t.Errorf("resource = %+v, want %+v", entry.Resources[0], want)
	}
}

func TestNewSkillEntryRejectsInvalidSkill(t *testing.T) {
	cases := map[string]string{
		"no frontmatter":        "# Title\n",
		"empty body":            "",
		"unterminated":          "---\nname: demo\ndescription: d\n",
		"name mismatch":         "---\nname: other\ndescription: d\n---\n",
		"missing description":   "---\nname: demo\n---\n",
		"nested mapping":        "---\nname: demo\ndescription: d\nmetadata:\n  a: b\n---\n",
		"quoted scalar":         "---\nname: demo\ndescription: \"d\"\n---\n",
		"block scalar":          "---\nname: demo\ndescription: >\n  d\n---\n",
		"indented key":          "---\nname: demo\n description: d\n---\n",
		"frontmatter not first": "\n---\nname: demo\ndescription: d\n---\n",
		"trailing comment":      "---\nname: demo\ndescription: d # note\n---\n",
		"duplicate key":         "---\nname: demo\ndescription: d\ndescription: e\n---\n",
		"boolean value":         "---\nname: demo\ndescription: true\n---\n",
		"null value":            "---\nname: demo\ndescription: null\n---\n",
		"numeric value":         "---\nname: demo\ndescription: 1.5\n---\n",
		"nested mapping value":  "---\nname: demo\ndescription: a: b\n---\n",
		"sequence value":        "---\nname: demo\ndescription: [a, b]\n---\n",
		"invalid key":           "---\nname: demo\ndescription: d\n? x: y\n---\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if entry, err := newSkillEntry("demo", "skill://demo/SKILL.md", []byte(body)); err == nil {
				t.Fatalf("expected an error, got %+v", entry)
			}
		})
	}
}

func TestParseSkillFrontmatterAcceptsCRLF(t *testing.T) {
	fields, err := parseSkillFrontmatter([]byte("---\r\nname: demo\r\ndescription: a/b (c), d\r\n---\r\nbody\r\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if fields["name"] != "demo" || fields["description"] != "a/b (c), d" {
		t.Errorf("fields = %v", fields)
	}
}

// TestSkillResourceRoundtrip pins that the skill is listed as a markdown resource
// named after its frontmatter and that reading it returns the embedded SKILL.md.
func TestSkillResourceRoundtrip(t *testing.T) {
	cs := newToolClient(t, AddSkills)
	ctx := context.Background()

	listed, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if len(listed.Resources) != 1 {
		t.Fatalf("resources = %+v, want only the skill", listed.Resources)
	}
	got := listed.Resources[0]
	if got.URI != usingSofaRPCSkillURI || got.Name != usingSofaRPCSkillName || got.MIMEType != "text/markdown" {
		t.Errorf("resource = %+v", got)
	}
	if !strings.HasPrefix(got.Description, "Use when invoking any SofaRPC service") {
		t.Errorf("description = %q", got.Description)
	}

	res, err := cs.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: usingSofaRPCSkillURI})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(res.Contents) != 1 || res.Contents[0].Text != string(skills.UsingSofaRPC) {
		t.Errorf("read did not return the embedded SKILL.md")
	}
	if res.Contents[0].MIMEType != "text/markdown" || res.Contents[0].URI != usingSofaRPCSkillURI {
		t.Errorf("contents = %s %s", res.Contents[0].URI, res.Contents[0].MIMEType)
	}
}
